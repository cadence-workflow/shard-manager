package spectatorclient

import (
	"context"
	"fmt"
	"sync"

	"github.com/cadence-workflow/shard-manager/common/types"
	csync "github.com/cadence-workflow/shard-manager/service/sharddistributor/client/spectatorclient/sync"
)

// registry is the namespace state the spectator routes on. The stream and the poller both update it.
type registry struct {
	mu sync.RWMutex
	// Map from shard ID to shard owner (executor ID + metadata)
	shardToOwner map[string]*ShardOwner
	// drainedShards is the local drain set
	drainedShards map[string]struct{}
	// applied is the assignment revision of the applied state, which the stream or the poller delivered
	applied int64
	// streamed is the assignment revision the current stream last delivered, -1 until it delivers
	streamed int64
	// ready is released once state has been applied
	ready *csync.ResettableSignal
}

func newRegistry() *registry {
	return &registry{ready: csync.NewResettableSignal()}
}

// applyStreamed applies state the stream delivered. The revision counts as delivered even when the
// state is refused, so a stream sending stale state still looks behind.
func (r *registry) applyStreamed(response *types.WatchNamespaceStateResponse) bool {
	shardToOwner, drainedShards := buildState(response)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.streamed = response.AssignmentRevision
	return r.replace(response.AssignmentRevision, shardToOwner, drainedShards)
}

func (r *registry) applyPolled(response *types.WatchNamespaceStateResponse) bool {
	shardToOwner, drainedShards := buildState(response)

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.replace(response.AssignmentRevision, shardToOwner, drainedShards)
}

// resetStreamed marks that the current stream has delivered nothing yet.
func (r *registry) resetStreamed() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.streamed = -1
}

// replace swaps in the state and reports whether it did. State older than the applied state is refused.
// Callers hold the lock.
func (r *registry) replace(revision int64, shardToOwner map[string]*ShardOwner, drainedShards map[string]struct{}) bool {
	// Refused state releases waiters too, since the held state is newer.
	r.ready.Done()
	// Old servers send no revision (0), so their state is always applied.
	if revision != 0 && revision < r.applied {
		return false
	}
	r.shardToOwner = shardToOwner
	r.drainedShards = drainedShards
	r.applied = revision
	return true
}

func buildState(response *types.WatchNamespaceStateResponse) (map[string]*ShardOwner, map[string]struct{}) {
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
	return shardToOwner, drainedShards
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

func (r *registry) appliedRevision() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.applied
}

// streamBehind reports whether the current stream has not delivered the polled assignment revision.
// State from an older server carries no revision and cannot be compared.
func (r *registry) streamBehind(polled int64) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.streamed != 0 && r.streamed < polled
}
