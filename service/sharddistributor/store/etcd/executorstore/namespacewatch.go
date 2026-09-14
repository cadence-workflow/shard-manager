package executorstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/cadence-workflow/shard-manager/common/backoff"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdclient"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdkeys"
)

const (
	// Namespace watch retries land between 50ms and 150ms apart.
	namespaceWatchRetryInterval = 100 * time.Millisecond
	namespaceWatchJitterCoeff   = 0.5
)

// namespaceWatcher turns etcd namespace watches into coalesced change signals. It owns
// the goroutines it starts: every subscription runs until Stop, which is the only
// background lifecycle the executor store has.
type namespaceWatcher struct {
	client        etcdclient.Client
	prefix        string
	logger        log.Logger
	timeSource    clock.TimeSource
	metricsClient metrics.Client

	// watchCtx bounds every watch started here. Stop cancels it, which closes the etcd
	// watch channels, and waits for those goroutines to exit.
	watchCtx    context.Context
	cancelWatch context.CancelFunc
	wg          sync.WaitGroup

	// mu makes starting a watch and shutting down mutually exclusive, so no
	// subscription can add to wg once Stop has committed to waiting on it.
	mu      sync.Mutex
	stopped bool
}

func newNamespaceWatcher(
	client etcdclient.Client,
	etcdCfg etcdclient.ExecutorStoreConfig,
	logger log.Logger,
	timeSource clock.TimeSource,
	metricsClient metrics.Client,
) *namespaceWatcher {
	watchCtx, cancelWatch := context.WithCancel(context.Background())

	return &namespaceWatcher{
		client:        client,
		prefix:        etcdCfg.Prefix,
		logger:        logger,
		timeSource:    timeSource,
		metricsClient: metricsClient,
		watchCtx:      watchCtx,
		cancelWatch:   cancelWatch,
	}
}

// Stop is idempotent: it is wired to shutdown, which may run after a failed start.
func (w *namespaceWatcher) Stop() {
	w.mu.Lock()
	w.stopped = true
	w.cancelWatch()
	w.mu.Unlock()

	// Waited for outside the lock so a watch goroutine can never block shutdown by
	// contending for it.
	w.wg.Wait()
}

// SubscribeToNamespaceChanges starts watching a namespace and returns the channel its
// changes are signalled on. The channel is closed once the watcher stops.
func (w *namespaceWatcher) SubscribeToNamespaceChanges(namespace string) (<-chan struct{}, error) {
	// Registered under the lock Stop takes. Testing watchCtx instead leaves a window:
	// Stop can cancel and reach wg.Wait between the check and the Add, which is a
	// WaitGroup reuse and leaves this watch unawaited.
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return nil, errors.New("store is stopping")
	}
	w.wg.Add(1)
	w.mu.Unlock()

	changeChan := make(chan struct{}, 1)
	go func() {
		defer w.wg.Done()
		defer close(changeChan)

		logger := w.logger.WithTags(tag.ShardNamespace(namespace))

		for {
			err := w.watchNamespace(namespace, changeChan)
			if w.watchCtx.Err() != nil {
				return
			}
			if err != nil {
				// A watch drops whenever its etcd node goes away, which is expected and
				// self-healing: the retry below re-establishes it.
				logger.Info("namespace watch failed, retrying", tag.Error(err))
			}

			select {
			case <-w.timeSource.After(backoff.JitDuration(
				namespaceWatchRetryInterval,
				namespaceWatchJitterCoeff,
			)):
			case <-w.watchCtx.Done():
				return
			}
		}
	}()

	return changeChan, nil
}

// watchNamespace feeds changeChan until the watch ends, returning the reason it did.
func (w *namespaceWatcher) watchNamespace(namespace string, changeChan chan<- struct{}) error {
	scope := w.metricsClient.Scope(metrics.ShardDistributorWatchScope).
		Tagged(metrics.NamespaceTag(namespace)).
		Tagged(metrics.ShardDistributorWatchTypeTag("namespace_state"))

	// The whole namespace is watched as one range so assignment, metadata and
	// drained-shard changes arrive in revision order on a single watch.
	watchChan := w.client.Watch(
		// WithRequireLeader ensures that the etcd cluster has a leader
		clientv3.WithRequireLeader(w.watchCtx),
		etcdkeys.BuildNamespacePrefix(w.prefix, namespace),
		clientv3.WithPrefix(),
		clientv3.WithPrevKV(),
	)

	// A reconnected watch resumes at the current revision, so anything written while
	// it was down is never delivered. Signalling here makes the subscriber re-read and
	// pick those changes up. It must follow the Watch call: a read the subscriber
	// starts before the watch exists could miss writes that land in between.
	w.signalNamespaceChange(changeChan)

	for watchResp := range watchChan {
		if err := watchResp.Err(); err != nil {
			return fmt.Errorf("watch response: %w", err)
		}

		sw := scope.StartTimer(metrics.ShardDistributorWatchProcessingLatency)
		scope.AddCounter(metrics.ShardDistributorWatchEventsReceived, int64(len(watchResp.Events)))

		if w.hasNamespaceStateChanged(watchResp, namespace) {
			w.signalNamespaceChange(changeChan)
		}
		sw.Stop()
	}

	return fmt.Errorf("watch channel closed")
}

// signalNamespaceChange nudges the subscriber to re-read. A pending signal already
// says that, so it is coalesced rather than blocking on a slow consumer.
func (w *namespaceWatcher) signalNamespaceChange(changeChan chan<- struct{}) {
	select {
	case changeChan <- struct{}{}:
	default:
	}
}

// hasNamespaceStateChanged reports whether a namespace watch response touched
// executor assignments, executor metadata, or the drained shard set.
func (w *namespaceWatcher) hasNamespaceStateChanged(watchResp clientv3.WatchResponse, namespace string) bool {
	executorsPrefix := etcdkeys.BuildExecutorsPrefix(w.prefix, namespace)
	drainedShardsPrefix := etcdkeys.BuildDrainedShardsPrefix(w.prefix, namespace)

	for _, event := range watchResp.Events {
		key := string(event.Kv.Key)

		switch {
		case strings.HasPrefix(key, executorsPrefix):
			_, keyType, err := etcdkeys.ParseExecutorKey(w.prefix, namespace, key)
			if err != nil {
				w.logger.Warn("Received watch event with unrecognized key format", tag.Key(key))
				continue
			}
			if keyType != etcdkeys.ExecutorAssignedStateKey && keyType != etcdkeys.ExecutorMetadataKey {
				continue
			}
			// Skip a rewrite of the same value.
			if event.PrevKv != nil && string(event.Kv.Value) == string(event.PrevKv.Value) {
				continue
			}
			return true

		case strings.HasPrefix(key, drainedShardsPrefix):
			// Every recognized drained key counts as a change: draining stores no value
			// and undraining arrives as a nil-valued tombstone, so a previous-value
			// comparison would see "" on both sides and miss the undrain.
			if _, err := etcdkeys.ParseDrainedShardKey(w.prefix, namespace, key); err != nil {
				w.logger.Warn("Received drained shards watch event with unrecognized key format", tag.Error(err))
				continue
			}
			return true
		}
	}
	return false
}
