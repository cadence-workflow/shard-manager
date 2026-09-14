package cache

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination=cache_mock.go ShardCache

import (
	"context"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// ShardCache answers shard and executor lookups from a cached view of the namespace,
// which it keeps current in the background from a Store. Lookups that miss the cache
// trigger a refresh, so a caller never sees a shard purely because the cache is cold.
type ShardCache interface {
	// GetShardOwner retrieves the owner of a specific shard within a namespace.
	// It returns store.ErrShardNotFound if the shard does not exist, and store.ErrShardDrained
	// if the shard is drained.
	GetShardOwner(ctx context.Context, namespace, shardID string) (*store.ShardOwner, error)

	// GetExecutor retrieves an executor within a namespace.
	GetExecutor(ctx context.Context, namespace string, executorID string) (*store.ShardOwner, error)

	// GetShardAssignments returns a snapshot of assignments and drained shards.
	GetShardAssignments(namespace string) (store.AssignmentSnapshot, error)

	// Subscribe signals when the namespace's assignments change. The returned
	// function unsubscribes.
	Subscribe(namespace string) (<-chan struct{}, func(), error)
}
