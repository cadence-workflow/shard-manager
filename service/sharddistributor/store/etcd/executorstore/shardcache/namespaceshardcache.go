package shardcache

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

const (
	// refreshSingleFlightKey is the shared singleflight key for cache-miss
	// triggered full refreshes.
	refreshSingleFlightKey = "refresh"

	// refreshOperationTimeout caps how long a singleflighted refresh can keep
	// the singleflight key occupied. The etcd client config
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
	stopCh           chan struct{}
	logger           log.Logger
	executorStore    store.Store
	timeSource       clock.TimeSource
	pubSub           *executorStatePubSub
	metricsClient    metrics.Client

	// refreshSF deduplicates concurrent cache-miss refreshes. When N callers
	// simultaneously miss the cache, only one of them performs the etcd read;
	// the rest wait on the shared result.
	refreshSF singleflight.Group
	// refreshTimeout bounds an in-flight refresh; defaults to
	// refreshOperationTimeout.
	refreshTimeout time.Duration
}

func newNamespaceShardToExecutor(namespace string, executorStore store.Store, stopCh chan struct{}, logger log.Logger, timeSource clock.TimeSource, metricsClient metrics.Client) *namespaceShardToExecutor {
	return &namespaceShardToExecutor{
		shardToExecutor:  make(map[string]*store.ShardOwner),
		executorToShards: make(map[*store.ShardOwner][]string),
		executorRevision: make(map[string]int64),
		shardOwners:      make(map[string]*store.ShardOwner),
		drainedShards:    make(map[string]struct{}),
		namespace:        namespace,
		stopCh:           stopCh,
		logger:           logger.WithTags(tag.ShardNamespace(namespace)),
		executorStore:    executorStore,
		timeSource:       timeSource,
		pubSub:           newExecutorStatePubSub(logger, namespace),
		metricsClient:    metricsClient,
		refreshTimeout:   refreshOperationTimeout,
	}
}

func (n *namespaceShardToExecutor) Start(wg *sync.WaitGroup) error {
	changeCh, err := n.executorStore.SubscribeToNamespaceChanges(n.namespace)
	if err != nil {
		return fmt.Errorf("subscribe to namespace changes: %w", err)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		n.namespaceRefreshLoop(changeCh)
	}()

	return nil
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

func (n *namespaceShardToExecutor) Subscribe() (<-chan struct{}, func()) {
	return n.pubSub.subscribe()
}

func (n *namespaceShardToExecutor) namespaceRefreshLoop(changeCh <-chan struct{}) {
	for {
		select {
		case <-n.stopCh:
			n.logger.Info("stop channel closed, exiting namespaceRefreshLoop")
			return

		case _, ok := <-changeCh:
			if !ok {
				n.logger.Info("namespace change channel closed, exiting namespaceRefreshLoop")
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

// refreshNamespaceState reads every keyspace the cache tracks in one store call, so both
// the executor state and the drained set advance together under a single store revision.
func (n *namespaceShardToExecutor) refreshNamespaceState(ctx context.Context) (bool, error) {
	state, err := n.executorStore.GetState(ctx, n.namespace)
	if err != nil {
		return false, fmt.Errorf("get state for namespace %s: %w", n.namespace, err)
	}

	return n.applyNamespaceState(state), nil
}

func (n *namespaceShardToExecutor) applyNamespaceState(state *store.NamespaceState) bool {
	shardToExecutor := make(map[string]*store.ShardOwner)
	executorState := make(map[*store.ShardOwner][]string)
	executorRevision := make(map[string]int64)
	shardOwners := make(map[string]*store.ShardOwner)

	for executorID, executor := range state.Executors {
		shardOwner := getOrCreateShardOwner(shardOwners, executorID)

		if assigned, ok := state.ShardAssignments[executorID]; ok {
			shardIDs := make([]string, 0, len(assigned.AssignedShards))
			for shardID := range assigned.AssignedShards {
				shardToExecutor[shardID] = shardOwner
				shardIDs = append(shardIDs, shardID)
			}
			executorState[shardOwner] = shardIDs
			executorRevision[executorID] = assigned.ModRevision
		}

		maps.Copy(shardOwner.Metadata, executor.Metadata)
	}

	return n.replaceNamespaceState(state.Revision, shardToExecutor, executorState, executorRevision, shardOwners, state.DrainedShards)
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
