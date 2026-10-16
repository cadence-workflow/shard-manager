package cache

import (
	"context"
	"fmt"
	"sync"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// ShardCache is the NamespaceCache of a single namespace. Its owner calls Start
// before serving lookups and Stop on shutdown.
type ShardCache struct {
	namespace *namespaceShardToExecutor
	stopC     chan struct{}
	wg        sync.WaitGroup
}

var _ NamespaceCache = (*ShardCache)(nil)

// NewShardCache builds the cache of one namespace.
func NewShardCache(namespace string, executorStore store.Store, logger log.Logger, timeSource clock.TimeSource, metricsClient metrics.Client) *ShardCache {
	stopC := make(chan struct{})
	return &ShardCache{
		namespace: newNamespaceShardToExecutor(namespace, executorStore, stopC, logger, timeSource, metricsClient),
		stopC:     stopC,
	}
}

// Start subscribes to namespace changes and begins the background refresh loop.
func (c *ShardCache) Start() error {
	return c.namespace.Start(&c.wg)
}

// Stop ends the refresh loop and waits for it to exit.
func (c *ShardCache) Stop() {
	close(c.stopC)
	c.wg.Wait()
}

// GetShardOwner returns the owner of the shard.
// Drained shards return ErrShardDrained rather than the last assigned owner
func (c *ShardCache) GetShardOwner(ctx context.Context, shardID string) (*store.ShardOwner, error) {
	drained, err := c.namespace.IsShardDrained(ctx, shardID)
	if err != nil {
		return nil, fmt.Errorf("check shard drained: %w", err)
	}
	if drained {
		return nil, store.ErrShardDrained
	}
	return c.namespace.GetShardOwner(ctx, shardID)
}

func (c *ShardCache) GetExecutor(ctx context.Context, executorID string) (*store.ShardOwner, error) {
	return c.namespace.GetExecutor(ctx, executorID)
}

func (c *ShardCache) GetShardAssignments() store.AssignmentSnapshot {
	return c.namespace.GetShardAssignments()
}

func (c *ShardCache) Subscribe() (<-chan struct{}, func()) {
	return c.namespace.Subscribe()
}
