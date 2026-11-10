package spectatorclient

import (
	"context"
	"fmt"
	"sync"

	"github.com/cadence-workflow/shard-manager/common/types"
	csync "github.com/cadence-workflow/shard-manager/service/sharddistributor/client/spectatorclient/sync"
)

// registry is the namespace state the spectator routes on.
type registry struct {
	mu sync.RWMutex
	// Map from shard ID to shard owner (executor ID + metadata)
	shardToOwner map[string]*ShardOwner
	// drainedShards is the local drain set
	drainedShards map[string]struct{}
	// ready is released once state has been applied
	ready *csync.ResettableSignal
}

func newRegistry() *registry {
	return &registry{ready: csync.NewResettableSignal()}
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
	r.ready.Done()
}

// lookup waits until state has been applied or the registry is closed.
func (r *registry) lookup(ctx context.Context, shardKey string) (owner *ShardOwner, drained bool, err error) {
	if err := r.ready.Wait(ctx); err != nil {
		return nil, false, fmt.Errorf("wait for first state: %w", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	_, drained = r.drainedShards[shardKey]
	return r.shardToOwner[shardKey], drained, nil
}

// resetReady makes lookup wait until state is applied again.
func (r *registry) resetReady() {
	r.ready.Reset()
}

// close releases waiters, since no state arrives once the spectator has stopped.
func (r *registry) close() {
	r.ready.Done()
}
