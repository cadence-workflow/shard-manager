package shardcache

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/fx"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/cache"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// ShardCacheParams defines the dependencies for the shard cache, for use with fx.
type ShardCacheParams struct {
	fx.In

	ExecutorStore store.Store
	Lifecycle     fx.Lifecycle
	Logger        log.Logger
	TimeSource    clock.TimeSource
	MetricsClient metrics.Client
}

type NamespaceToShards map[string]*namespaceShardToExecutor
type ShardToExecutorCache struct {
	sync.RWMutex
	namespaceToShards NamespaceToShards
	timeSource        clock.TimeSource
	executorStore     store.Store
	stopC             chan struct{}
	logger            log.Logger
	wg                sync.WaitGroup
	metricsClient     metrics.Client
}

// Module provides the shard cache to the fx application.
var Module = fx.Module("shardcache",
	fx.Provide(NewShardCache),
)

func NewShardCache(p ShardCacheParams) cache.ShardCache {
	cache := &ShardToExecutorCache{
		namespaceToShards: make(NamespaceToShards),
		timeSource:        p.TimeSource,
		executorStore:     p.ExecutorStore,
		stopC:             make(chan struct{}),
		logger:            p.Logger,
		wg:                sync.WaitGroup{},
		metricsClient:     p.MetricsClient,
	}

	p.Lifecycle.Append(fx.StartStopHook(cache.Start, cache.Stop))

	return cache
}

func (s *ShardToExecutorCache) Start() {}

func (s *ShardToExecutorCache) Stop() {
	close(s.stopC)
	s.wg.Wait()
}

// GetShardOwner returns the owner of the shard.
// Drained shards return ErrShardDrained rather than the last assigned owner
func (s *ShardToExecutorCache) GetShardOwner(ctx context.Context, namespace, shardID string) (*store.ShardOwner, error) {
	drained, err := s.IsShardDrained(ctx, namespace, shardID)
	if err != nil {
		return nil, fmt.Errorf("check shard drained: %w", err)
	}
	if drained {
		return nil, store.ErrShardDrained
	}

	namespaceShardToExecutor, err := s.getNamespaceShardToExecutor(namespace)
	if err != nil {
		return nil, fmt.Errorf("get namespace shard to executor: %w", err)
	}
	return namespaceShardToExecutor.GetShardOwner(ctx, shardID)
}

// IsShardDrained reports whether the shard marked as drained
func (s *ShardToExecutorCache) IsShardDrained(ctx context.Context, namespace, shardID string) (bool, error) {
	namespaceShardToExecutor, err := s.getNamespaceShardToExecutor(namespace)
	if err != nil {
		return false, fmt.Errorf("get namespace shard to executor: %w", err)
	}
	return namespaceShardToExecutor.IsShardDrained(ctx, shardID)
}

func (s *ShardToExecutorCache) GetExecutor(ctx context.Context, namespace, executorID string) (*store.ShardOwner, error) {
	namespaceShardToExecutor, err := s.getNamespaceShardToExecutor(namespace)
	if err != nil {
		return nil, fmt.Errorf("get namespace shard to executor: %w", err)
	}
	return namespaceShardToExecutor.GetExecutor(ctx, executorID)
}

func (s *ShardToExecutorCache) Subscribe(namespace string) (<-chan struct{}, func(), error) {
	namespaceShardToExecutor, err := s.getNamespaceShardToExecutor(namespace)
	if err != nil {
		return nil, nil, fmt.Errorf("get namespace shard to executor: %w", err)
	}

	ch, unSub := namespaceShardToExecutor.Subscribe()
	return ch, unSub, nil
}

func (s *ShardToExecutorCache) GetShardAssignments(namespace string) (store.AssignmentSnapshot, error) {
	namespaceShardToExecutor, err := s.getNamespaceShardToExecutor(namespace)
	if err != nil {
		return store.AssignmentSnapshot{}, fmt.Errorf("get namespace shard to executor: %w", err)
	}
	return namespaceShardToExecutor.GetShardAssignments(), nil
}

func (s *ShardToExecutorCache) getNamespaceShardToExecutor(namespace string) (*namespaceShardToExecutor, error) {
	s.RLock()
	namespaceShardToExecutor, ok := s.namespaceToShards[namespace]
	s.RUnlock()

	if ok {
		return namespaceShardToExecutor, nil
	}

	s.Lock()
	defer s.Unlock()

	if namespaceShardToExecutor, ok := s.namespaceToShards[namespace]; ok {
		return namespaceShardToExecutor, nil
	}

	namespaceShardToExecutor = newNamespaceShardToExecutor(namespace, s.executorStore, s.stopC, s.logger, s.timeSource, s.metricsClient)
	if err := namespaceShardToExecutor.Start(&s.wg); err != nil {
		return nil, fmt.Errorf("start namespace shard to executor: %w", err)
	}

	s.namespaceToShards[namespace] = namespaceShardToExecutor
	return namespaceShardToExecutor, nil
}
