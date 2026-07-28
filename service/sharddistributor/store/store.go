package store

import (
	"context"
	"fmt"
)

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination=store_mock.go Store
//go:generate gowrap gen -g -p . -i Store -t ./wrappers/templates/metered.tmpl -o ./wrappers/metered/store_generated.go -v handler=Wrapped

var (
	// ErrExecutorNotFound is an error that is returned when queries executor is not registered in the storage.
	ErrExecutorNotFound = fmt.Errorf("executor not found")

	// ErrShardNotFound is an error that is returned when a shard does not exist.
	ErrShardNotFound = fmt.Errorf("shard not found")

	// ErrVersionConflict is an error that is returned if during operations some precondition failed.
	ErrVersionConflict = fmt.Errorf("version conflict")

	// ErrExecutorNotRunning is an error that is returned when shard is attempted to be assigned to a not running executor.
	ErrExecutorNotRunning = fmt.Errorf("executor not running")
)

type ErrShardAlreadyAssigned struct {
	ShardID    string
	AssignedTo string
	Metadata   map[string]string
}

func (e *ErrShardAlreadyAssigned) Error() string {
	return fmt.Sprintf("shard %s is already assigned to %s", e.ShardID, e.AssignedTo)
}

// Txn represents a generic, backend-agnostic transaction.
// It is used as a vehicle for the GuardFunc to operate on.
type Txn interface{}

// GuardFunc is a function that applies a transactional precondition.
// It takes a generic transaction, applies a backend-specific guard,
// and returns the modified transaction.
type GuardFunc func(Txn) (Txn, error)

// NopGuard is a no-op guard that can be used when no transactional
// check is required. It simply returns the transaction as-is.
func NopGuard() GuardFunc {
	return func(txn Txn) (Txn, error) {
		return txn, nil
	}
}

// AssignShardsRequest is a request to assign shards to executors, and remove unused shards.
type AssignShardsRequest struct {
	// NewState is the new state of the namespace, containing the new assignments of shards to executors.
	NewState *NamespaceState
	// ExecutorsToDelete maps executor IDs to their expected ModRevision for deletion.
	// The ModRevision is used to ensure the executor's assigned state hasn't changed since we decided to delete it.
	ExecutorsToDelete map[string]int64
	// ChangedExecutors is the set of executor IDs whose assigned state actually
	// changed and need to be written. All executors in NewState.ShardAssignments
	// are still included in the transactional ModRevision guard, but only those
	// in ChangedExecutors get an OpPut. If nil, all executors are written
	// (backwards-compatible default for callers that don't track deltas).
	ChangedExecutors map[string]struct{}
}

type Store interface {
	// GetState retrieves the current state of a namespace, including executors,
	// shard statistics, and shard assignments.
	GetState(ctx context.Context, namespace string) (*NamespaceState, error)

	// AssignShards assigns multiple shards to executors within a namespace.
	// It also updates shard statistics and deletes specified executors
	// The operation is atomic and guarded by the provided GuardFunc.
	AssignShards(ctx context.Context, namespace string, request AssignShardsRequest, guard GuardFunc) error

	// AssignShard assigns a single shard to an executor within a namespace.
	AssignShard(ctx context.Context, namespace string, shardID string, executorID string) error

	// SubscribeToExecutorStatusChanges subscribes to changes of executors' status key within a namespace.
	SubscribeToExecutorStatusChanges(ctx context.Context, namespace string) (<-chan int64, error)
	DeleteExecutors(ctx context.Context, namespace string, executorIDs []string, guard GuardFunc) error

	// DeleteAssignedStates deletes the assigned states of multiple executors within a namespace.
	DeleteAssignedStates(ctx context.Context, namespace string, executorIDs []string, guard GuardFunc) error

	// GetShardOwner retrieves the owner of a specific shard within a namespace.
	// It returns ErrShardNotFound if the shard does not exist.
	GetShardOwner(ctx context.Context, namespace, shardID string) (*ShardOwner, error)
	SubscribeToAssignmentChanges(ctx context.Context, namespace string) (<-chan map[*ShardOwner][]string, func(), error)

	// GetExecutor retrieves an executor within a namespace.
	GetExecutor(ctx context.Context, namespace string, executorID string) (*ShardOwner, error)

	GetHeartbeat(ctx context.Context, namespace string, executorID string) (*HeartbeatState, *AssignedState, error)
	RecordHeartbeat(ctx context.Context, namespace, executorID string, state HeartbeatState) error

	DeleteShardStats(ctx context.Context, namespace string, shardIDs []string, guard GuardFunc) error

	// ResetNamespace deletes every key under the namespace prefix in storage,
	// including the leader key, executor heartbeats/status/metadata, shard
	// assignments, and shard statistics. The namespace itself stays configured
	// at the service-config level; executors will re-register on their next
	// heartbeat and the next leader will rebuild the assignment plan.
	//
	// This is intentionally NOT guarded by leadership: any in-flight leader
	// writes will fail their own leadership guard once the leader key is gone,
	// which is the desired outcome.
	//
	// Returns the number of keys removed by the underlying delete.
	ResetNamespace(ctx context.Context, namespace string) (int64, error)
}
