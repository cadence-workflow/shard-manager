package store

import (
	"time"

	"github.com/cadence-workflow/shard-manager/common/types"
)

type HeartbeatState struct {
	// LastHeartbeat is the time of the last heartbeat received from the executor
	LastHeartbeat  time.Time
	Status         types.ExecutorStatus
	ReportedShards map[string]*types.ShardStatusReport
	Metadata       map[string]string
}

type AssignedState struct {
	// AssignedShards holds the current assignment of shards to this executor
	// Key: ShardID
	AssignedShards map[string]*types.ShardAssignment

	// ShardHandoverStats holds handover statistics of all shards experienced handovers to this executor
	// Mostly all shards in AssignedShards will have corresponding entries here
	// But if a shard was assigned but never had a handover (e.g., first assignment), it does not have an entry here
	// Key: ShardID
	ShardHandoverStats map[string]ShardHandoverStats

	// LastUpdated is the time when this assignment state was last updated
	// Used to calculate assignment distribution latency for newly assigned shards
	LastUpdated time.Time
	ModRevision int64
}

// ShardHandoverStats holds statistics related to the latest handover of a shard
type ShardHandoverStats struct {
	// PreviousExecutorLastHeartbeatTime is the last heartbeat time received
	// from the previous executor before the shard was reassigned.
	PreviousExecutorLastHeartbeatTime time.Time

	// HandoverType indicates the type of handover that occurred during the last shard reassignment.
	HandoverType types.HandoverType
}

type NamespaceState struct {
	// Executors holds the heartbeat states of all executors in the namespace.
	// Key: ExecutorID
	Executors map[string]HeartbeatState

	// ShardStats holds the statistics of all shards in the namespace.
	// Only loaded for namespace which types.LoadBalancingMode is types.LoadBalancingModeGREEDY
	// Key: ShardID
	ShardStats map[string]ShardStatistics

	// ShardAssignments holds the assignment states of all shards in the namespace.
	// Key: ExecutorID
	ShardAssignments map[string]AssignedState

	// DrainedShards holds the shards that are drained for this namespace.
	// A drained shard is not eligible for assignment until it is
	// explicitly undrained.
	// Key: ShardID. Value is the time the shard was first marked drained.
	// A zero time means the shard is drained but the start time is unknown
	// (legacy empty etcd values).
	DrainedShards map[string]time.Time
}

type ShardState struct {
	ExecutorID string
}

type ShardStatistics struct {
	// Exponential weighted moving average of shard load that persists across executor changes
	SmoothedLoad float64

	// LastUpdateTime is the heartbeat timestamp that last updated the smoothed load.
	// Zero means the shard has never been measured. Should not be set at assignment.
	LastUpdateTime time.Time

	// LastMoveTime is the timestamp when this shard was last reassigned
	LastMoveTime time.Time
}

type ShardOwner struct {
	ExecutorID string
	Metadata   map[string]string
}

// CountExecutorsByStatus returns a map of executor status to the count of executors with that status
func (ns *NamespaceState) CountExecutorsByStatus() map[types.ExecutorStatus]int {
	counts := make(map[types.ExecutorStatus]int)
	for _, executor := range ns.Executors {
		counts[executor.Status]++
	}
	return counts
}

// ShardOwners flattens the per-executor assignments into a shardID -> executorID lookup
func (ns *NamespaceState) ShardOwners() map[string]string {
	owners := make(map[string]string)
	for executorID, assigned := range ns.ShardAssignments {
		for shardID := range assigned.AssignedShards {
			owners[shardID] = executorID
		}
	}
	return owners
}

// IsDrained reports whether the shard is currently drained, regardless of
// whether its drain time is known.
func (ns *NamespaceState) IsDrained(shardID string) bool {
	_, drained := ns.DrainedShards[shardID]
	return drained
}

// DrainedSince returns when the shard was first marked drained. The second
// result is false when the shard is not drained, or when it is drained but its
// drain time is unknown.
func (ns *NamespaceState) DrainedSince(shardID string) (time.Time, bool) {
	drainedAt, ok := ns.DrainedShards[shardID]
	if !ok || drainedAt.IsZero() {
		return time.Time{}, false
	}
	return drainedAt, true
}

// DrainedShardIDs returns the drained shards as a set, for callers that only
// need membership.
func (ns *NamespaceState) DrainedShardIDs() map[string]struct{} {
	ids := make(map[string]struct{}, len(ns.DrainedShards))
	for shardID := range ns.DrainedShards {
		ids[shardID] = struct{}{}
	}
	return ids
}

// OldestDrainAge returns how long the longest-draining shard has been drained.
// Shards whose drain time is unknown do not contribute, so a namespace holding
// only those reports zero rather than an inflated age.
func (ns *NamespaceState) OldestDrainAge(now time.Time) time.Duration {
	var oldest time.Time
	for _, drainedAt := range ns.DrainedShards {
		if drainedAt.IsZero() {
			continue
		}
		if oldest.IsZero() || drainedAt.Before(oldest) {
			oldest = drainedAt
		}
	}
	if oldest.IsZero() {
		return 0
	}
	if age := now.Sub(oldest); age > 0 {
		return age
	}
	return 0
}
