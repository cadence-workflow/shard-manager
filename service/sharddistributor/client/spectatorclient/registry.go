package spectatorclient

import (
	"sync"

	"github.com/cadence-workflow/shard-manager/common/types"
)

// registry is the namespace state the spectator routes on.
type registry struct {
	mu sync.RWMutex
	// Map from shard ID to shard owner (executor ID + metadata)
	shardToOwner map[string]*ShardOwner
	// drainedShards is the local drain set
	drainedShards map[string]struct{}
}

// apply replaces the state with the response.
func (r *registry) apply(response *types.WatchNamespaceStateResponse) {
	// Build inverted map: shard ID -> shard owner (executor ID + metadata)
	shardToOwner := make(map[string]*ShardOwner)
	for _, executor := range response.Executors {
		owner := &ShardOwner{
			ExecutorID: executor.ExecutorID,
			Metadata:   executor.Metadata,
		}
		for _, shard := range executor.AssignedShards {
			shardToOwner[shard.ShardKey] = owner
		}
	}

	drainedShards := make(map[string]struct{}, len(response.DrainedShardKeys))
	for _, shardKey := range response.DrainedShardKeys {
		drainedShards[shardKey] = struct{}{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.shardToOwner = shardToOwner
	r.drainedShards = drainedShards
}

func (r *registry) lookup(shardKey string) (owner *ShardOwner, drained bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, drained = r.drainedShards[shardKey]
	return r.shardToOwner[shardKey], drained
}
