package cache

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/fx"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/namespaces"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// ShardCacheParams defines the dependencies for the shard cache, for use with fx.
type ShardCacheParams struct {
	fx.In

	Namespaces    *namespaces.Registry
	ExecutorStore store.Store
	Lifecycle     fx.Lifecycle
	Logger        log.Logger
	TimeSource    clock.TimeSource
	MetricsClient metrics.Client
}

type NamespaceToShards map[string]*namespaceShardToExecutor

// ShardToExecutorCache holds one cache per configured namespace.
// namespaceToShards is populated at construction and never modified afterwards,
// so it is read without locking.
type ShardToExecutorCache struct {
	namespaceToShards NamespaceToShards
	stopC             chan struct{}
	wg                sync.WaitGroup
}

// Module provides the shard cache to the fx application.
var Module = fx.Module("shardcache",
	fx.Provide(NewShardCache),
)

func NewShardCache(p ShardCacheParams) ShardCache {
	cache := &ShardToExecutorCache{
		namespaceToShards: make(NamespaceToShards),
		stopC:             make(chan struct{}),
	}
	for _, ns := range p.Namespaces.List() {
		cache.namespaceToShards[ns.Name] = newNamespaceShardToExecutor(ns.Name, p.ExecutorStore, cache.stopC, p.Logger, p.TimeSource, p.MetricsClient)
	}

	p.Lifecycle.Append(fx.StartStopHook(cache.Start, cache.Stop))

	return cache
}

// Start begins the refresh loop of every namespace. On failure, the loops
// that were already started are stopped before returning.
func (s *ShardToExecutorCache) Start() error {
	for name, namespaceShardToExecutor := range s.namespaceToShards {
		if err := namespaceShardToExecutor.Start(&s.wg); err != nil {
			s.Stop()
			return fmt.Errorf("start shard cache for namespace %s: %w", name, err)
		}
	}
	return nil
}

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
	namespaceShardToExecutor, ok := s.namespaceToShards[namespace]
	if !ok {
		return nil, &types.NamespaceNotFoundError{Namespace: namespace}
	}
	return namespaceShardToExecutor, nil
}
