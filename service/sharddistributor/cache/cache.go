package cache

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination=cache_mock.go ShardCache

import (
	"context"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// ShardCache answers shard and executor lookups of one namespace from a cached
// view, which it keeps current in the background from a Store. Lookups that miss the
// cache trigger a refresh, so a caller never misses a shard purely because the cache
// is cold.
type ShardCache interface {
	// Start subscribes to namespace changes and begins the background refresh.
	// It must be called before serving lookups.
	Start() error

	// Stop ends the background refresh and waits for it to exit.
	Stop()

	// GetShardOwner retrieves the owner of a shard.
	// It returns store.ErrShardNotFound if the shard does not exist, and store.ErrShardDrained
	// if the shard is drained.
	GetShardOwner(ctx context.Context, shardID string) (*store.ShardOwner, error)

	// GetExecutor retrieves an executor.
	GetExecutor(ctx context.Context, executorID string) (*store.ShardOwner, error)

	// GetShardAssignments returns a snapshot of assignments and drained shards.
	GetShardAssignments() store.AssignmentSnapshot

	// Subscribe signals when assignments, executor metadata or drained shards
	// change. The returned function unsubscribes.
	Subscribe() (<-chan struct{}, func())
}
