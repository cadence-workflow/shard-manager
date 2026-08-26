package shardcache

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"golang.org/x/sync/singleflight"

	"github.com/cadence-workflow/shard-manager/common/backoff"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdclient"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdkeys"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdtypes"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/executorstore/common"
)

const (
	// RetryInterval for watch failures is between 50ms to 150ms
	namespaceRefreshLoopWatchJitterCoeff   = 0.5
	namespaceRefreshLoopWatchRetryInterval = 100 * time.Millisecond

	// refreshSingleFlightKey is the shared singleflight key for cache-miss
	// triggered full refreshes.
	refreshSingleFlightKey = "refresh"

	// refreshOperationTimeout caps how long a singleflighted refresh / stats
	// fetch can keep the singleflight key occupied. The etcd client config
	// only sets DialTimeout, so without this bound a hung Get would block
	// every joiner indefinitely (until stopCh closes).
	refreshOperationTimeout = 5 * time.Second
)

type namespaceShardToExecutor struct {
	sync.RWMutex

	shardToExecutor  map[string]*store.ShardOwner   // shardID -> shardOwner
	shardOwners      map[string]*store.ShardOwner   // executorID -> shardOwner
	executorToShards map[*store.ShardOwner][]string // executor -> shardIDs
	executorRevision map[string]int64
	drainedShards    map[string]struct{} // set of shard IDs marked drained
	lastRevision     int64               // etcd store revision of the last applied snapshot
	namespace        string
	etcdPrefix       string
	stopCh           chan struct{}
	logger           log.Logger
	client           etcdclient.Client
	timeSource       clock.TimeSource
	pubSub           *executorStatePubSub
	metricsClient    metrics.Client

	// namespacePrefix is watched and read as one range, and the keyspaces below it are
	// partitioned by the prefixes that follow
	namespacePrefix     string
	executorsPrefix     string
	drainedShardsPrefix string

	// refreshSF deduplicates concurrent cache-miss refreshes. When N callers
	// simultaneously miss the cache, only one of them performs the etcd read;
	// the rest wait on the shared result.
	refreshSF singleflight.Group
	// statsSF deduplicates concurrent per-executor statistics cache misses.
	statsSF singleflight.Group
	// refreshTimeout bounds an in-flight refresh / stats fetch; defaults to
	// refreshOperationTimeout.
	refreshTimeout time.Duration

	executorStatistics *namespaceExecutorStatistics
}

type namespaceExecutorStatistics struct {
	sync.RWMutex
	stats map[string]map[string]etcdtypes.ShardStatistics // executorID -> shardID -> ShardStatistics
}

func newNamespaceExecutorStatistics() *namespaceExecutorStatistics {
	return &namespaceExecutorStatistics{
		stats: make(map[string]map[string]etcdtypes.ShardStatistics),
	}
}

func newNamespaceShardToExecutor(etcdPrefix, namespace string, client etcdclient.Client, stopCh chan struct{}, logger log.Logger, timeSource clock.TimeSource, metricsClient metrics.Client) (*namespaceShardToExecutor, error) {
	return &namespaceShardToExecutor{
		shardToExecutor:     make(map[string]*store.ShardOwner),
		executorToShards:    make(map[*store.ShardOwner][]string),
		executorRevision:    make(map[string]int64),
		shardOwners:         make(map[string]*store.ShardOwner),
		drainedShards:       make(map[string]struct{}),
		namespace:           namespace,
		etcdPrefix:          etcdPrefix,
		namespacePrefix:     etcdkeys.BuildNamespacePrefix(etcdPrefix, namespace),
		executorsPrefix:     etcdkeys.BuildExecutorsPrefix(etcdPrefix, namespace),
		drainedShardsPrefix: etcdkeys.BuildDrainedShardsPrefix(etcdPrefix, namespace),
		stopCh:              stopCh,
		logger:              logger.WithTags(tag.ShardNamespace(namespace)),
		client:              client,
		timeSource:          timeSource,
		pubSub:              newExecutorStatePubSub(logger, namespace),
		executorStatistics:  newNamespaceExecutorStatistics(),
		metricsClient:       metricsClient,
		refreshTimeout:      refreshOperationTimeout,
	}, nil
}

func (n *namespaceShardToExecutor) Start(wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		n.namespaceRefreshLoop()
	}()
}

func (n *namespaceShardToExecutor) GetShardOwner(ctx context.Context, shardID string) (*store.ShardOwner, error) {
	shardOwner, err := n.getShardOwnerInMap(ctx, &n.shardToExecutor, shardID)
	if err != nil {
		return nil, fmt.Errorf("get shard owner in map: %w", err)
	}
	if shardOwner != nil {
		return shardOwner, nil
	}

	return nil, store.ErrShardNotFound
}

// IsShardDrained answers from the cached drained set, which the namespace watch keeps current
func (n *namespaceShardToExecutor) IsShardDrained(ctx context.Context, shardID string) (bool, error) {
	if drained, loaded := n.lookupDrained(shardID); loaded {
		return drained, nil
	}

	if err := n.refreshSingleFlight(ctx); err != nil {
		return false, fmt.Errorf("refresh for namespace %s: %w", n.namespace, err)
	}

	drained, _ := n.lookupDrained(shardID)
	return drained, nil
}

// lookupDrained reports whether the shard is drained, and whether the cache holds a
// loaded snapshot at all
func (n *namespaceShardToExecutor) lookupDrained(shardID string) (drained, loaded bool) {
	n.RLock()
	defer n.RUnlock()

	_, drained = n.drainedShards[shardID]
	return drained, n.lastRevision > 0
}

func (n *namespaceShardToExecutor) GetExecutor(ctx context.Context, executorID string) (*store.ShardOwner, error) {
	shardOwner, err := n.getShardOwnerInMap(ctx, &n.shardOwners, executorID)
	if err != nil {
		return nil, fmt.Errorf("get shard owner in map: %w", err)
	}
	if shardOwner != nil {
		return shardOwner, nil
	}

	return nil, store.ErrExecutorNotFound
}

func (n *namespaceShardToExecutor) GetExecutorModRevisionCmp() ([]clientv3.Cmp, error) {
	n.RLock()
	defer n.RUnlock()
	comparisons := []clientv3.Cmp{}
	for executor, revision := range n.executorRevision {
		executorAssignedStateKey := etcdkeys.BuildExecutorKey(n.etcdPrefix, n.namespace, executor, etcdkeys.ExecutorAssignedStateKey)
		comparisons = append(comparisons, clientv3.Compare(clientv3.ModRevision(executorAssignedStateKey), "=", revision))
	}

	return comparisons, nil
}

func (n *namespaceShardToExecutor) GetExecutorStatistics(ctx context.Context, executorID string) (map[string]etcdtypes.ShardStatistics, error) {
	if stats, found := n.getStats(executorID); found {
		return stats, nil
	}

	if err := n.populateExecutorStatisticsCacheOnMiss(ctx, executorID); err != nil {
		return nil, err
	}

	// Refreshing cache after cache miss should allow the statistics to be found
	if stats, found := n.getStats(executorID); found {
		return stats, nil
	}

	return nil, fmt.Errorf("executor statistics not found after refresh: %w", store.ErrExecutorNotFound)
}

func (n *namespaceShardToExecutor) getStats(executorID string) (map[string]etcdtypes.ShardStatistics, bool) {
	n.executorStatistics.RLock()
	defer n.executorStatistics.RUnlock()

	stats, ok := n.executorStatistics.stats[executorID]
	if ok {
		return maps.Clone(stats), true
	}

	return nil, false
}

// populateExecutorStatisticsCacheOnMiss fetches executor statistics from etcd
// and caches them, deduplicating concurrent misses for the same executor.
func (n *namespaceShardToExecutor) populateExecutorStatisticsCacheOnMiss(ctx context.Context, executorID string) error {
	ch := n.statsSF.DoChan(executorID, func() (interface{}, error) {
		fetchCtx, cancel := context.WithTimeout(context.Background(), n.refreshTimeout)
		defer cancel()
		return nil, n.fetchAndCacheExecutorStatistics(fetchCtx, executorID)
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-ch:
		return res.Err
	}
}

func (n *namespaceShardToExecutor) fetchAndCacheExecutorStatistics(ctx context.Context, executorID string) error {
	statsKey := etcdkeys.BuildExecutorKey(n.etcdPrefix, n.namespace, executorID, etcdkeys.ExecutorShardStatisticsKey)
	resp, err := n.client.Get(ctx, statsKey)
	if err != nil {
		return fmt.Errorf("get executor shard statistics: %w", err)
	}

	n.executorStatistics.Lock()
	defer n.executorStatistics.Unlock()

	// The watch loop may have populated the cache while reading etcd.
	// In that case, do not overwrite the newer cached value.
	if _, ok := n.executorStatistics.stats[executorID]; ok {
		return nil
	}

	if len(resp.Kvs) > 0 {
		stats := make(map[string]etcdtypes.ShardStatistics)
		if err := common.DecompressAndUnmarshal(resp.Kvs[0].Value, &stats); err != nil {
			return fmt.Errorf("parse executor shard statistics: %w", err)
		}
		n.executorStatistics.stats[executorID] = stats
	} else {
		return store.ErrExecutorNotFound
	}
	return nil
}

func (n *namespaceShardToExecutor) Subscribe() (<-chan struct{}, func()) {
	return n.pubSub.subscribe()
}

func (n *namespaceShardToExecutor) namespaceRefreshLoop() {
	triggerCh, watcherDone := n.runWatchLoop()

	// The watcher applies statistics updates straight to the cache, so returning here
	// would release the caller's WaitGroup while those writes are still possible.
	defer func() { <-watcherDone }()

	for {
		select {
		case <-n.stopCh:
			n.logger.Info("stop channel closed, exiting namespaceRefreshLoop")
			return

		case _, ok := <-triggerCh:
			if !ok {
				n.logger.Info("trigger channel closed, exiting namespaceRefreshLoop")
				return
			}

			// Bounded so a stuck etcd call can't block this loop from observing stopCh.
			refreshCtx, cancel := context.WithTimeout(context.Background(), n.refreshTimeout)
			err := n.refresh(refreshCtx)
			cancel()
			if err != nil {
				n.logger.Error("failed to refresh namespace shard to executor", tag.Error(err))
			}
		}
	}
}

// runWatchLoop starts the watcher and returns the refresh trigger channel plus a
// closed channel once the watcher has stopped
func (n *namespaceShardToExecutor) runWatchLoop() (<-chan struct{}, <-chan struct{}) {
	triggerCh := make(chan struct{}, 1)
	watcherDone := make(chan struct{})

	go func() {
		defer close(watcherDone)
		defer close(triggerCh)

		for {
			if err := n.watch(triggerCh); err != nil {
				n.logger.Error("error watching in namespaceRefreshLoop, retrying...", tag.Error(err))

				// The backoff observes stopCh so a pending retry interval cannot
				// hold up shutdown, and cannot strand the watcher when the clock
				// is mocked.
				select {
				case <-n.timeSource.After(backoff.JitDuration(
					namespaceRefreshLoopWatchRetryInterval,
					namespaceRefreshLoopWatchJitterCoeff,
				)):
					continue
				case <-n.stopCh:
					n.logger.Info("stop channel closed during watch retry backoff, exiting watch loop")
					return
				}
			}

			n.logger.Info("namespaceRefreshLoop is exiting")
			return
		}
	}()

	return triggerCh, watcherDone
}

func (n *namespaceShardToExecutor) watch(triggerCh chan<- struct{}) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scope := n.metricsClient.Scope(metrics.ShardDistributorWatchScope).
		Tagged(metrics.NamespaceTag(n.namespace)).
		Tagged(metrics.ShardDistributorWatchTypeTag("cache_refresh"))

	watchChan := n.client.Watch(
		// WithRequireLeader ensures that the etcd cluster has a leader
		clientv3.WithRequireLeader(ctx),
		n.namespacePrefix,
		clientv3.WithPrefix(),
		clientv3.WithPrevKV(),
	)

	for {
		select {
		case <-n.stopCh:
			n.logger.Info("stop channel closed, exiting watch loop")
			return nil

		case watchResp, ok := <-watchChan:
			if err := watchResp.Err(); err != nil {
				return fmt.Errorf("watch response: %w", err)
			}
			if !ok {
				return fmt.Errorf("watch channel closed")
			}

			// Track watch metrics
			sw := scope.StartTimer(metrics.ShardDistributorWatchProcessingLatency)
			scope.AddCounter(metrics.ShardDistributorWatchEventsReceived, int64(len(watchResp.Events)))

			if !n.needsRefresh(watchResp) {
				sw.Stop()
				continue
			}

			select {
			case triggerCh <- struct{}{}:
			default:
				n.logger.Info("Cache is being refreshed, skipping trigger")
			}
			sw.Stop()
		}
	}
}

// needsRefresh checks whether a watch response over the namespace prefix requires a
// cache refresh. Both checks run before the result is combined, because
// hasExecutorStateChanged also applies statistics events as a side effect and
// short-circuiting on a drained-shard change would drop them.
func (n *namespaceShardToExecutor) needsRefresh(watchResp clientv3.WatchResponse) bool {
	executorStateChanged := n.hasExecutorStateChanged(watchResp)
	drainedShardsChanged := n.hasDrainedShardsChanged(watchResp)
	return executorStateChanged || drainedShardsChanged
}

// hasDrainedShardsChanged checks whether any event touched the drained shards keyspace.
// Every recognized key counts as a change: draining stores no value and an undrain arrives
// as a tombstone with a nil value, so the previous-value comparison used for executor keys
// sees "" on both sides and would count the undrain as an unchanged key.
func (n *namespaceShardToExecutor) hasDrainedShardsChanged(watchResp clientv3.WatchResponse) bool {
	changed := false
	for _, event := range watchResp.Events {
		key := string(event.Kv.Key)
		if !strings.HasPrefix(key, n.drainedShardsPrefix) {
			continue
		}

		if _, err := etcdkeys.ParseDrainedShardKey(n.etcdPrefix, n.namespace, key); err != nil {
			n.logger.Warn("Received drained shards watch event with unrecognized key format", tag.Error(err))
			continue
		}
		changed = true
	}
	return changed
}

// hasExecutorStateChanged checks if any of the events in the watch response indicate a change to executor assigned state or metadata,
// and if the value actually changed (not just same value written again)
func (n *namespaceShardToExecutor) hasExecutorStateChanged(watchResp clientv3.WatchResponse) bool {
	needsRefresh := false
	for _, event := range watchResp.Events {
		// The watch spans the whole namespace, so events from sibling keyspaces such as
		// drained shards and leader election arrive here and are not executor keys.
		if !strings.HasPrefix(string(event.Kv.Key), n.executorsPrefix) {
			continue
		}

		executorID, keyType, keyErr := etcdkeys.ParseExecutorKey(n.etcdPrefix, n.namespace, string(event.Kv.Key))
		if keyErr != nil {
			n.logger.Warn("Received watch event with unrecognized key format", tag.Value(keyErr))
			continue
		}

		// Check if value actually changed (skip if same value written again)
		if event.PrevKv != nil && string(event.Kv.Value) == string(event.PrevKv.Value) {
			continue
		}

		switch keyType {
		case etcdkeys.ExecutorShardStatisticsKey:
			n.handleExecutorStatisticsEvent(executorID, *event)
		case etcdkeys.ExecutorAssignedStateKey, etcdkeys.ExecutorMetadataKey:
			needsRefresh = true
		}
	}
	return needsRefresh
}

func (n *namespaceShardToExecutor) refresh(ctx context.Context) error {
	updated, err := n.refreshNamespaceState(ctx)
	if err != nil {
		return fmt.Errorf("refresh namespace state: %w", err)
	}

	if updated {
		n.pubSub.notifySubscribers()
	}
	return nil
}

func (n *namespaceShardToExecutor) GetShardAssignments() store.AssignmentSnapshot {
	n.RLock()
	defer n.RUnlock()

	shardAssignments := make(map[*store.ShardOwner][]string, len(n.executorToShards))
	for executor, shardIDs := range n.executorToShards {
		shardAssignments[executor] = slices.Clone(shardIDs)
	}

	return store.AssignmentSnapshot{
		ExecutorToShards: shardAssignments,
		DrainedShards:    maps.Clone(n.drainedShards),
	}
}

// refreshNamespaceState reads every keyspace the cache tracks in one range read, so both
// the executor state and the drained set advance together under a single etcd revision.
func (n *namespaceShardToExecutor) refreshNamespaceState(ctx context.Context) (bool, error) {
	resp, err := n.client.Get(ctx, n.namespacePrefix, clientv3.WithPrefix())
	if err != nil {
		return false, fmt.Errorf("get namespace prefix for namespace %s: %w", n.namespace, err)
	}

	executorKVs, drainedShards := n.partitionNamespaceKVs(resp.Kvs)

	parsedData, err := common.ParseExecutorKVs(n.etcdPrefix, n.namespace, executorKVs)
	if err != nil {
		return false, fmt.Errorf("failed to parse executor data: %w", err)
	}

	updated := n.applyNamespaceData(resp.Header.Revision, parsedData, drainedShards)
	return updated, nil
}

// partitionNamespaceKVs splits a namespace range read into the executor keys, which need
// full parsing, and the set of drained shard IDs. Keys belonging to any other sibling
// keyspace, leader election being the only one today, are ignored.
func (n *namespaceShardToExecutor) partitionNamespaceKVs(kvs []*mvccpb.KeyValue) ([]*mvccpb.KeyValue, map[string]struct{}) {
	executorKVs := make([]*mvccpb.KeyValue, 0, len(kvs))
	drainedShards := make(map[string]struct{})

	for _, kv := range kvs {
		key := string(kv.Key)

		switch {
		case strings.HasPrefix(key, n.executorsPrefix):
			executorKVs = append(executorKVs, kv)
		case strings.HasPrefix(key, n.drainedShardsPrefix):
			shardID, err := etcdkeys.ParseDrainedShardKey(n.etcdPrefix, n.namespace, key)
			if err != nil {
				// A single malformed key must not fail every drain lookup in the namespace.
				n.logger.Warn("Skipping drained shard key with unrecognized format", tag.Error(err))
				continue
			}
			drainedShards[shardID] = struct{}{}
		}
	}

	return executorKVs, drainedShards
}

func (n *namespaceShardToExecutor) applyNamespaceData(storeRevision int64, executors map[string]*etcdtypes.ParsedExecutorData, drainedShards map[string]struct{}) bool {
	shardToExecutor := make(map[string]*store.ShardOwner)
	executorState := make(map[*store.ShardOwner][]string)
	executorRevision := make(map[string]int64)
	shardOwners := make(map[string]*store.ShardOwner)
	executorStatistics := make(map[string]map[string]etcdtypes.ShardStatistics)

	for executorID, executorData := range executors {
		shardOwner := getOrCreateShardOwner(shardOwners, executorID)
		if executorData.AssignedState != nil {
			shardIDs := make([]string, 0, len(executorData.AssignedState.AssignedShards))
			for shardID := range executorData.AssignedState.AssignedShards {
				shardToExecutor[shardID] = shardOwner
				shardIDs = append(shardIDs, shardID)
			}
			executorState[shardOwner] = shardIDs
			executorRevision[executorID] = executorData.AssignedState.ModRevision
		}

		maps.Copy(shardOwner.Metadata, executorData.Metadata)

		executorStatistics[executorID] = maps.Clone(executorData.Statistics)
	}

	updated := n.replaceNamespaceState(storeRevision, shardToExecutor, executorState, executorRevision, shardOwners, drainedShards)
	n.executorStatistics.replaceStatistics(executorStatistics)
	return updated
}

func (n *namespaceShardToExecutor) replaceNamespaceState(
	storeRevision int64,
	shardToExecutor map[string]*store.ShardOwner,
	executorState map[*store.ShardOwner][]string,
	executorRevision map[string]int64,
	shardOwners map[string]*store.ShardOwner,
	drainedShards map[string]struct{},
) bool {
	n.Lock()
	defer n.Unlock()

	if storeRevision <= n.lastRevision {
		n.logger.Debug("skipping stale cache update",
			tag.Dynamic("storeRevision", storeRevision),
			tag.Dynamic("lastRevision", n.lastRevision),
		)
		return false
	}

	n.lastRevision = storeRevision
	n.shardToExecutor = shardToExecutor
	n.executorToShards = executorState
	n.executorRevision = executorRevision
	n.shardOwners = shardOwners
	n.drainedShards = drainedShards
	return true
}

// handleExecutorStatisticsEvent processes incoming watch events for executor shard statistics.
// It updates the in-memory statistics map directly from the event without triggering a full refresh.
func (n *namespaceShardToExecutor) handleExecutorStatisticsEvent(executorID string, event clientv3.Event) {
	if event.Type == clientv3.EventTypeDelete {
		n.executorStatistics.deleteStatistics(executorID)
		return
	}

	stats := make(map[string]etcdtypes.ShardStatistics)
	if err := common.DecompressAndUnmarshal(event.Kv.Value, &stats); err != nil {
		n.logger.Error(
			"failed to parse executor statistics from watch event",
			tag.ShardNamespace(n.namespace),
			tag.ShardExecutor(executorID),
			tag.Error(err),
		)
		return
	}

	n.executorStatistics.assignStatistics(executorID, stats)
}

func (n *namespaceExecutorStatistics) deleteStatistics(executorID string) {
	n.Lock()
	defer n.Unlock()
	delete(n.stats, executorID)
}

func (n *namespaceExecutorStatistics) assignStatistics(executorID string, stats map[string]etcdtypes.ShardStatistics) {
	n.Lock()
	defer n.Unlock()
	n.stats[executorID] = maps.Clone(stats)
}

func (n *namespaceExecutorStatistics) replaceStatistics(stats map[string]map[string]etcdtypes.ShardStatistics) {
	n.Lock()
	defer n.Unlock()
	n.stats = stats
}

// getOrCreateShardOwner retrieves an existing ShardOwner from the map or creates a new one if it doesn't exist
func getOrCreateShardOwner(shardOwners map[string]*store.ShardOwner, executorID string) *store.ShardOwner {
	shardOwner, ok := shardOwners[executorID]
	if !ok {
		shardOwner = &store.ShardOwner{
			ExecutorID: executorID,
			Metadata:   make(map[string]string),
		}
		shardOwners[executorID] = shardOwner
	}
	return shardOwner
}

// getShardOwnerInMap retrieves a shard owner from the map if it exists, otherwise it refreshes the cache and tries again
// it takes a pointer to the map. When the cache is refreshed, the map is updated, so we need to pass a pointer to the map
func (n *namespaceShardToExecutor) getShardOwnerInMap(ctx context.Context, m *map[string]*store.ShardOwner, key string) (*store.ShardOwner, error) {
	n.RLock()
	shardOwner, ok := (*m)[key]
	n.RUnlock()
	if ok {
		return shardOwner, nil
	}

	if err := n.refreshSingleFlight(ctx); err != nil {
		return nil, fmt.Errorf("refresh for namespace %s: %w", n.namespace, err)
	}

	n.RLock()
	shardOwner, ok = (*m)[key]
	n.RUnlock()
	if ok {
		return shardOwner, nil
	}
	return nil, nil
}

// refreshSingleFlight collapses concurrent cache-miss refreshes into a single
// etcd read.
// The refresh runs under a fresh background context bounded by
// refreshTimeout: detached so the leader's cancellation cannot poison the
// flight for joiners, bounded, so a hung etcd read cannot hold the singleflight
// key indefinitely.
func (n *namespaceShardToExecutor) refreshSingleFlight(ctx context.Context) error {
	ch := n.refreshSF.DoChan(refreshSingleFlightKey, func() (interface{}, error) {
		refreshCtx, cancel := context.WithTimeout(context.Background(), n.refreshTimeout)
		defer cancel()
		return nil, n.refresh(refreshCtx)
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-ch:
		return res.Err
	}
}
