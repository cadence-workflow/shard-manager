package store

import (
	"strings"
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

// ExecutorState contains the persisted state for one executor.
type ExecutorState struct {
	Heartbeat  *HeartbeatState
	Assignment *AssignedState
	Statistics map[string]ShardStatistics
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
	// Key: ShardID
	ShardStats map[string]ShardStatistics

	// ShardAssignments holds the assignment states of all shards in the namespace.
	// Key: ExecutorID
	ShardAssignments map[string]AssignedState

	// DrainedShards holds the shards that are drained for this namespace.
	// A drained shard is not eligible for assignment until it is
	// explicitly undrained.
	// Key: ShardID
	DrainedShards map[string]struct{}

	// DrainedHosts holds host drains for this namespace.
	// A drained host's executors are not eligible for assignment until
	// the host is explicitly undrained
	DrainedHosts map[string]DrainedHost
}

// DrainedHost is the persisted metadata for a host drain
type DrainedHost struct {
	Hostname  string    `json:"hostname"`
	DrainedAt time.Time `json:"drained_at"`
	DrainedBy string    `json:"drained_by,omitempty"`
	Reason    string    `json:"reason,omitempty"`
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

// ExecutorShardStatistics contains the complete shard statistics map for one executor.
type ExecutorShardStatistics struct {
	ExecutorID string
	Statistics map[string]ShardStatistics
}

type ShardOwner struct {
	ExecutorID string
	Metadata   map[string]string
}

type AssignmentSnapshot struct {
	ExecutorToShards map[*ShardOwner][]string
	DrainedShards    map[string]struct{}
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

func (ns *NamespaceState) IsShardDrained(shardID string) bool {
	_, drained := ns.DrainedShards[shardID]
	return drained
}

func (ns *NamespaceState) IsHostDrained(hostname string) bool {
	_, drained := ns.DrainedHosts[hostname]
	return drained
}

// ExecutorHost returns the host portion of an executor ID (hostname@uuid).
// IDs without '@' are treated as a bare hostname.
func ExecutorHost(executorID string) string {
	if i := strings.LastIndex(executorID, "@"); i >= 0 {
		return executorID[:i]
	}
	return executorID
}

// OverlayOperatorDrain sets PERMANENTLY_DRAINED on executors whose host is in
// DrainedHosts. This is a read-time overlay; persisted heartbeat status is unchanged.
func OverlayOperatorDrain(state *NamespaceState) {
	if state == nil || len(state.DrainedHosts) == 0 {
		return
	}
	for id, heartbeat := range state.Executors {
		if state.IsHostDrained(ExecutorHost(id)) {
			heartbeat.Status = types.ExecutorStatusPERMANENTLY_DRAINED
			state.Executors[id] = heartbeat
		}
	}
}
