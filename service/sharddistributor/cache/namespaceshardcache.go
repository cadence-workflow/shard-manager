package cache

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
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

type shardCache struct {
	sync.RWMutex

	shardToExecutor  map[string]*store.ShardOwner   // shardID -> shardOwner
	shardOwners      map[string]*store.ShardOwner   // executorID -> shardOwner
	executorToShards map[*store.ShardOwner][]string // executor -> shardIDs
	executorRevision map[string]int64
	drainedShards    map[string]struct{} // set of shard IDs marked drained
	lastRevision     int64               // etcd store revision of the last applied snapshot
	namespace        string
	stopCh           chan struct{}
	wg               sync.WaitGroup
	logger           log.Logger
	executorStore    store.Store
	pubSub           *executorStatePubSub

	// refreshSF deduplicates concurrent cache-miss refreshes. When N callers
	// simultaneously miss the cache, only one of them performs the etcd read;
	// the rest wait on the shared result.
	refreshSF singleflight.Group
	// refreshTimeout bounds an in-flight refresh; defaults to
	// refreshOperationTimeout.
	refreshTimeout time.Duration
}

var _ ShardCache = (*shardCache)(nil)

// NewShardCache builds the cache of one namespace.
func NewShardCache(namespace string, executorStore store.Store, logger log.Logger) ShardCache {
	return newShardCache(namespace, executorStore, logger)
}

func newShardCache(namespace string, executorStore store.Store, logger log.Logger) *shardCache {
	return &shardCache{
		shardToExecutor:  make(map[string]*store.ShardOwner),
		executorToShards: make(map[*store.ShardOwner][]string),
		executorRevision: make(map[string]int64),
		shardOwners:      make(map[string]*store.ShardOwner),
		drainedShards:    make(map[string]struct{}),
		namespace:        namespace,
		stopCh:           make(chan struct{}),
		logger:           logger.WithTags(tag.ShardNamespace(namespace)),
		executorStore:    executorStore,
		pubSub:           newExecutorStatePubSub(),
		refreshTimeout:   refreshOperationTimeout,
	}
}

func (c *shardCache) Start() error {
	changeCh, err := c.executorStore.SubscribeToNamespaceChanges(c.namespace)
	if err != nil {
		return fmt.Errorf("subscribe to namespace changes: %w", err)
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.namespaceRefreshLoop(changeCh)
	}()

	return nil
}

func (c *shardCache) Stop() {
	close(c.stopCh)
	c.wg.Wait()
}

// GetShardOwner returns the owner of the shard.
// Drained shards return ErrShardDrained rather than the last assigned owner
func (c *shardCache) GetShardOwner(ctx context.Context, shardID string) (*store.ShardOwner, error) {
	drained, err := c.IsShardDrained(ctx, shardID)
	if err != nil {
		return nil, fmt.Errorf("check shard drained: %w", err)
	}
	if drained {
		return nil, store.ErrShardDrained
	}

	shardOwner, err := c.getShardOwnerInMap(ctx, &c.shardToExecutor, shardID)
	if err != nil {
		return nil, fmt.Errorf("get shard owner in map: %w", err)
	}
	if shardOwner != nil {
		return shardOwner, nil
	}

	return nil, store.ErrShardNotFound
}

// IsShardDrained answers from the cached drained set, which the namespace watch keeps current
func (c *shardCache) IsShardDrained(ctx context.Context, shardID string) (bool, error) {
	if drained, loaded := c.lookupDrained(shardID); loaded {
		return drained, nil
	}

	if err := c.refreshSingleFlight(ctx); err != nil {
		return false, fmt.Errorf("refresh for namespace %s: %w", c.namespace, err)
	}

	drained, _ := c.lookupDrained(shardID)
	return drained, nil
}

// lookupDrained reports whether the shard is drained, and whether the cache holds a
// loaded snapshot at all
func (c *shardCache) lookupDrained(shardID string) (drained, loaded bool) {
	c.RLock()
	defer c.RUnlock()

	_, drained = c.drainedShards[shardID]
	return drained, c.lastRevision > 0
}

func (c *shardCache) GetExecutor(ctx context.Context, executorID string) (*store.ShardOwner, error) {
	shardOwner, err := c.getShardOwnerInMap(ctx, &c.shardOwners, executorID)
	if err != nil {
		return nil, fmt.Errorf("get shard owner in map: %w", err)
	}
	if shardOwner != nil {
		return shardOwner, nil
	}

	return nil, store.ErrExecutorNotFound
}

func (c *shardCache) Subscribe() (<-chan struct{}, func()) {
	return c.pubSub.subscribe()
}

func (c *shardCache) namespaceRefreshLoop(changeCh <-chan struct{}) {
	for {
		select {
		case <-c.stopCh:
			c.logger.Info("stop channel closed, exiting namespaceRefreshLoop")
			return

		case _, ok := <-changeCh:
			if !ok {
				c.logger.Info("namespace change channel closed, exiting namespaceRefreshLoop")
				return
			}

			// Bounded so a stuck etcd call can't block this loop from observing stopCh.
			refreshCtx, cancel := context.WithTimeout(context.Background(), c.refreshTimeout)
			err := c.refresh(refreshCtx)
			cancel()
			if err != nil {
				c.logger.Error("failed to refresh namespace shard to executor", tag.Error(err))
			}
		}
	}
}

func (c *shardCache) refresh(ctx context.Context) error {
	updated, err := c.refreshNamespaceState(ctx)
	if err != nil {
		return fmt.Errorf("refresh namespace state: %w", err)
	}

	if updated {
		c.pubSub.notifySubscribers()
	}
	return nil
}

func (c *shardCache) GetShardAssignments() store.AssignmentSnapshot {
	c.RLock()
	defer c.RUnlock()

	shardAssignments := make(map[*store.ShardOwner][]string, len(c.executorToShards))
	for executor, shardIDs := range c.executorToShards {
		shardAssignments[executor] = slices.Clone(shardIDs)
	}

	return store.AssignmentSnapshot{
		ExecutorToShards: shardAssignments,
		DrainedShards:    maps.Clone(c.drainedShards),
	}
}

// refreshNamespaceState reads every keyspace the cache tracks in one store call, so both
// the executor state and the drained set advance together under a single store revision.
func (c *shardCache) refreshNamespaceState(ctx context.Context) (bool, error) {
	state, err := c.executorStore.GetAssignmentState(ctx, c.namespace)
	if err != nil {
		return false, fmt.Errorf("get assignment state for namespace %s: %w", c.namespace, err)
	}

	return c.applyNamespaceState(state), nil
}

func (c *shardCache) applyNamespaceState(state *store.AssignmentState) bool {
	shardToExecutor := make(map[string]*store.ShardOwner)
	executorState := make(map[*store.ShardOwner][]string)
	executorRevision := make(map[string]int64)
	shardOwners := make(map[string]*store.ShardOwner)

	for executorID, executorMetadata := range state.ExecutorMetadata {
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

		maps.Copy(shardOwner.Metadata, executorMetadata)
	}

	return c.replaceNamespaceState(state.Revision, shardToExecutor, executorState, executorRevision, shardOwners, state.DrainedShards)
}

func (c *shardCache) replaceNamespaceState(
	storeRevision int64,
	shardToExecutor map[string]*store.ShardOwner,
	executorState map[*store.ShardOwner][]string,
	executorRevision map[string]int64,
	shardOwners map[string]*store.ShardOwner,
	drainedShards map[string]struct{},
) bool {
	c.Lock()
	defer c.Unlock()

	if storeRevision <= c.lastRevision {
		c.logger.Debug("skipping stale cache update",
			tag.Dynamic("storeRevision", storeRevision),
			tag.Dynamic("lastRevision", c.lastRevision),
		)
		return false
	}

	c.lastRevision = storeRevision
	c.shardToExecutor = shardToExecutor
	c.executorToShards = executorState
	c.executorRevision = executorRevision
	c.shardOwners = shardOwners
	c.drainedShards = drainedShards
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
func (c *shardCache) getShardOwnerInMap(ctx context.Context, m *map[string]*store.ShardOwner, key string) (*store.ShardOwner, error) {
	c.RLock()
	shardOwner, ok := (*m)[key]
	c.RUnlock()
	if ok {
		return shardOwner, nil
	}

	if err := c.refreshSingleFlight(ctx); err != nil {
		return nil, fmt.Errorf("refresh for namespace %s: %w", c.namespace, err)
	}

	c.RLock()
	shardOwner, ok = (*m)[key]
	c.RUnlock()
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
func (c *shardCache) refreshSingleFlight(ctx context.Context) error {
	ch := c.refreshSF.DoChan(refreshSingleFlightKey, func() (interface{}, error) {
		refreshCtx, cancel := context.WithTimeout(context.Background(), c.refreshTimeout)
		defer cancel()
		return nil, c.refresh(refreshCtx)
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-ch:
		return res.Err
	}
}
