package executorstore

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/fx/fxtest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdclient"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdkeys"
)

var _watchTestConfig = etcdclient.ExecutorStoreConfig{
	BaseConfig: etcdclient.BaseConfig{Prefix: _watchTestPrefix},
}

const (
	_watchTestPrefix    = "/test-prefix"
	_watchTestNamespace = "test-namespace"
	_watchTestExecutor  = "executor-1"
)

// newTestNamespaceWatcher builds a watcher wired to a mock etcd client.
func newTestNamespaceWatcher(t *testing.T, client etcdclient.Client, timeSource clock.TimeSource) *namespaceWatcher {
	t.Helper()

	w := newNamespaceWatcher(client, _watchTestConfig, testlogger.New(t), timeSource, metrics.NewNoopMetricsClient())
	t.Cleanup(w.cancelWatch)
	return w
}

func put(key, value string) *clientv3.Event {
	return &clientv3.Event{Type: clientv3.EventTypePut, Kv: &mvccpb.KeyValue{Key: []byte(key), Value: []byte(value)}}
}

// rewrite is a put over an existing key, which is only a change if the value differs.
func rewrite(key, prevValue, value string) *clientv3.Event {
	event := put(key, value)
	event.PrevKv = &mvccpb.KeyValue{Key: []byte(key), Value: []byte(prevValue)}
	return event
}

func del(key string) *clientv3.Event {
	return &clientv3.Event{
		Type:   clientv3.EventTypeDelete,
		Kv:     &mvccpb.KeyValue{Key: []byte(key)},
		PrevKv: &mvccpb.KeyValue{Key: []byte(key)},
	}
}

// The namespace is watched as a single range, so the store must refresh for drain changes,
// ignore the keyspaces subscribers do not track, and treat unchanged rewrites as no-ops.
func TestHasNamespaceStateChanged(t *testing.T) {
	w := newTestNamespaceWatcher(t, etcdclient.NewMockClient(gomock.NewController(t)), clock.NewRealTimeSource())

	drainedKey := etcdkeys.BuildDrainedShardKey(_watchTestPrefix, _watchTestNamespace, "shard-1")
	leaderKey := fmt.Sprintf("%s/%s/leader/1234", _watchTestPrefix, _watchTestNamespace)
	assignedStateKey := etcdkeys.BuildExecutorKey(_watchTestPrefix, _watchTestNamespace, _watchTestExecutor, etcdkeys.ExecutorAssignedStateKey)
	reportedShardsKey := etcdkeys.BuildExecutorKey(_watchTestPrefix, _watchTestNamespace, _watchTestExecutor, etcdkeys.ExecutorReportedShardsKey)
	metadataKey := etcdkeys.BuildMetadataKey(_watchTestPrefix, _watchTestNamespace, _watchTestExecutor, "metadata-key")

	tests := []struct {
		name     string
		events   []*clientv3.Event
		expected bool
	}{
		{
			name:     "no events",
			events:   nil,
			expected: false,
		},
		{
			name:     "shard drained",
			events:   []*clientv3.Event{put(drainedKey, "")},
			expected: true,
		},
		{
			// Drain and undrain both store an empty value, so this must not be mistaken
			// for an unchanged key.
			name:     "shard undrained",
			events:   []*clientv3.Event{del(drainedKey)},
			expected: true,
		},
		{
			name:     "leader election key is not tracked",
			events:   []*clientv3.Event{put(leaderKey, "host-1")},
			expected: false,
		},
		{
			name:     "malformed drained shard key",
			events:   []*clientv3.Event{put(etcdkeys.BuildDrainedShardsPrefix(_watchTestPrefix, _watchTestNamespace)+"shard/1", "")},
			expected: false,
		},
		{
			name:     "executor assigned state changed",
			events:   []*clientv3.Event{rewrite(assignedStateKey, "old", "new")},
			expected: true,
		},
		{
			name:     "executor assigned state rewritten with same value",
			events:   []*clientv3.Event{rewrite(assignedStateKey, "same", "same")},
			expected: false,
		},
		{
			// Heartbeat traffic is the highest-volume write in the namespace and no
			// subscriber reads it, so it must never cost a re-read.
			name:     "executor reported shards is not tracked",
			events:   []*clientv3.Event{put(reportedShardsKey, "reported")},
			expected: false,
		},
		{
			name:     "executor metadata changed",
			events:   []*clientv3.Event{rewrite(metadataKey, "old", "new")},
			expected: true,
		},
		{
			name:     "drain alongside an untracked key",
			events:   []*clientv3.Event{put(leaderKey, "host-1"), put(drainedKey, "")},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, w.hasNamespaceStateChanged(clientv3.WatchResponse{Events: tt.events}, _watchTestNamespace))
		})
	}
}

func TestWatchNamespace_ReturnsWatchFailures(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := etcdclient.NewMockClient(ctrl)

	watchChan := make(chan clientv3.WatchResponse)
	mockClient.EXPECT().Watch(gomock.Any(), gomock.Any(), gomock.Any()).Return(watchChan).AnyTimes()

	w := newTestNamespaceWatcher(t, mockClient, clock.NewRealTimeSource())
	changeChan := make(chan struct{}, 1)

	go func() {
		watchChan <- clientv3.WatchResponse{CompactRevision: 100}
	}()
	err := w.watchNamespace(_watchTestNamespace, changeChan)
	require.Error(t, err)
	assert.ErrorContains(t, err, "etcdserver: mvcc: required revision has been compacted")

	close(watchChan)
	err = w.watchNamespace(_watchTestNamespace, changeChan)
	require.Error(t, err)
	assert.ErrorContains(t, err, "watch channel closed")
}

// The signal channel holds one pending notification, so a subscriber that has not
// drained it must not be able to stall the watch loop.
func TestWatchNamespace_SignalsOnEstablishAndNeverBlocks(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctrl := gomock.NewController(t)
	mockClient := etcdclient.NewMockClient(ctrl)

	watchChan := make(chan clientv3.WatchResponse)
	mockClient.EXPECT().Watch(gomock.Any(), gomock.Any(), gomock.Any()).Return(watchChan).AnyTimes()

	w := newTestNamespaceWatcher(t, mockClient, clock.NewRealTimeSource())
	changeChan := make(chan struct{}, 1)

	watchDone := make(chan error, 1)
	go func() {
		watchDone <- w.watchNamespace(_watchTestNamespace, changeChan)
	}()

	// A reconnected watch resumes at the current revision, so establishing one has to
	// signal on its own for the subscriber to pick up what it missed.
	select {
	case <-changeChan:
	case <-time.After(time.Second):
		t.Fatal("establishing the watch should signal the subscriber")
	}

	assignedStateKey := etcdkeys.BuildExecutorKey(_watchTestPrefix, _watchTestNamespace, _watchTestExecutor, etcdkeys.ExecutorAssignedStateKey)
	for i := 0; i < 100; i++ {
		select {
		case watchChan <- clientv3.WatchResponse{Events: []*clientv3.Event{
			{Type: clientv3.EventTypePut, Kv: &mvccpb.KeyValue{Key: []byte(assignedStateKey), Value: []byte(fmt.Sprintf("v%d", i))}},
		}}:
		case <-time.After(time.Second):
			t.Fatal("watch loop is stuck - could not send event to watchChan")
		}
	}

	close(watchChan)
	select {
	case err := <-watchDone:
		assert.ErrorContains(t, err, "watch channel closed")
	case <-time.After(time.Second):
		t.Fatal("watch loop did not exit after the watch channel closed")
	}
}

// Subscribing once the store is stopping must fail rather than hand back a channel
// that closes immediately, which a subscriber cannot tell from a quiet namespace.
func TestSubscribeToNamespaceChanges_AfterStop(t *testing.T) {
	w := newTestNamespaceWatcher(t, etcdclient.NewMockClient(gomock.NewController(t)), clock.NewRealTimeSource())
	w.Stop()

	changeChan, err := w.SubscribeToNamespaceChanges(_watchTestNamespace)
	assert.Nil(t, changeChan)
	assert.ErrorContains(t, err, "store is stopping")
}

// Subscribe and Stop must not race on the WaitGroup: an Add that lands after Stop has
// committed to waiting is a reuse, which either panics or leaves the watch unawaited.
func TestNamespaceWatcher_SubscribeRacesStop(t *testing.T) {
	mockClient := etcdclient.NewMockClient(gomock.NewController(t))
	closed := make(chan clientv3.WatchResponse)
	close(closed)
	mockClient.EXPECT().Watch(gomock.Any(), gomock.Any(), gomock.Any()).
		Return((clientv3.WatchChan)(closed)).AnyTimes()

	w := newTestNamespaceWatcher(t, mockClient, clock.NewRealTimeSource())

	start := make(chan struct{})
	var callers sync.WaitGroup
	for i := 0; i < 32; i++ {
		callers.Add(1)
		go func() {
			defer callers.Done()
			<-start
			_, _ = w.SubscribeToNamespaceChanges(_watchTestNamespace)
		}()
	}
	callers.Add(1)
	go func() {
		defer callers.Done()
		<-start
		w.Stop()
	}()

	close(start)
	callers.Wait()
}

// The watcher registers its own shutdown, so fx stopping the app must end every watch
// it started without anything else calling Stop.
func TestNamespaceWatcher_StopsOnLifecycleShutdown(t *testing.T) {
	defer goleak.VerifyNone(t)

	mockClient := etcdclient.NewMockClient(gomock.NewController(t))
	// etcd closes the watch channel when its context is cancelled; the mock must too,
	// or the watch goroutine could never exit.
	mockClient.EXPECT().Watch(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, _ ...clientv3.OpOption) clientv3.WatchChan {
			ch := make(chan clientv3.WatchResponse)
			go func() {
				<-ctx.Done()
				close(ch)
			}()
			return ch
		}).AnyTimes()

	lifecycle := fxtest.NewLifecycle(t)
	w := provideNamespaceWatcher(NamespaceWatcherParams{
		Client:        mockClient,
		ETCDConfig:    _watchTestConfig,
		Lifecycle:     lifecycle,
		Logger:        testlogger.New(t),
		TimeSource:    clock.NewRealTimeSource(),
		MetricsClient: metrics.NewNoopMetricsClient(),
	})

	changeChan, err := w.SubscribeToNamespaceChanges(_watchTestNamespace)
	require.NoError(t, err)
	lifecycle.RequireStart()

	lifecycle.RequireStop()

	for range changeChan {
	}
}

// A failed watch must be retried rather than ending the subscription, and the
// retry loop must exit once the store stops.
func TestSubscribeToNamespaceChanges_RetriesFailedWatch(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctrl := gomock.NewController(t)
	mockClient := etcdclient.NewMockClient(ctrl)
	timeSource := clock.NewMockedTimeSource()

	watchChanRcvErr := make(chan clientv3.WatchResponse)
	mockClient.EXPECT().Watch(gomock.Any(), gomock.Any(), gomock.Any()).Return(watchChanRcvErr)

	watchChanClosed := make(chan clientv3.WatchResponse)
	mockClient.EXPECT().Watch(gomock.Any(), gomock.Any(), gomock.Any()).Return(watchChanClosed)

	// A third watch may or may not be established, depending on whether the store
	// stops before the retry interval elapses.
	mockClient.EXPECT().Watch(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(make(chan clientv3.WatchResponse)).
		MinTimes(0).
		MaxTimes(1)

	w := newTestNamespaceWatcher(t, mockClient, timeSource)

	changeChan, err := w.SubscribeToNamespaceChanges(_watchTestNamespace)
	require.NoError(t, err)

	closed := atomic.Bool{}
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for range changeChan {
		}
		closed.Store(true)
	}()

	// A compact revision makes WatchResponse.Err() non-nil, failing the watch.
	watchChanRcvErr <- clientv3.WatchResponse{CompactRevision: 100}
	timeSource.BlockUntil(1)
	require.False(t, closed.Load(), "a failed watch must not end the subscription")

	timeSource.Advance(2 * namespaceWatchRetryInterval)
	close(watchChanClosed)
	timeSource.BlockUntil(1)
	require.False(t, closed.Load(), "a closed watch channel must not end the subscription")

	w.Stop()
	<-drained
	assert.True(t, closed.Load(), "stopping the store must close the change channel")
}
