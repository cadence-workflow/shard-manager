package store

import (
	"slices"
	"time"

	"github.com/cadence-workflow/shard-manager/common/types"
)

type HeartbeatState struct {
	// LastHeartbeat is the time of the last heartbeat received from the executor
	LastHeartbeat  time.Time
	Status         types.ExecutorStatus
	ReportedShards map[string]*types.ShardStatusReport
	Metadata       map[string]string
	HostMetadata   *types.HostMetadata
}

func (h HeartbeatState) Hostname() string {
	if h.HostMetadata == nil {
		return ""
	}
	return h.HostMetadata.HostName
}

// Executor is a read-only view of one executor, derived from a NamespaceState
type Executor struct {
	ID          string
	Heartbeat   HeartbeatState
	HostDrained bool
}

// Status returns the executor status from its last heartbeat
func (e Executor) Status() types.ExecutorStatus {
	return e.Heartbeat.Status
}

func (e Executor) Hostname() string {
	return e.Heartbeat.Hostname()
}

func (e Executor) IsAssignable(staleExecutors map[string]int64) bool {
	if e.Status() != types.ExecutorStatusACTIVE || e.HostDrained {
		return false
	}
	_, stale := staleExecutors[e.ID]
	return !stale
}

func (e Executor) IsDraining() bool {
	status := e.Status()
	return status == types.ExecutorStatusDRAINING || status == types.ExecutorStatusDRAINED || e.HostDrained
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

// NamespaceState is the full snapshot of a namespace: who owns which shards, plus the
// heartbeats, statistics and host drains that only a rebalance needs.
type NamespaceState struct {
	AssignmentState

	// Executors holds the heartbeat states of all executors in the namespace.
	// Key: ExecutorID
	Executors map[string]HeartbeatState

	// ShardStats holds the statistics of all shards in the namespace.
	// Key: ShardID
	ShardStats map[string]ShardStatistics

	// DrainedHosts holds host drains for this namespace.
	// A drained host's executors are not eligible for assignment until
	// the host is explicitly undrained
	DrainedHosts map[string]DrainedHost
}

// AssignmentState is who owns which shards in a namespace, and which shards are
// drained. More elaborate information is available in the NamespaceState.
type AssignmentState struct {
	// ExecutorMetadata holds every executor in the namespace, assigned or not.
	// Key: ExecutorID, then metadata key
	ExecutorMetadata map[string]map[string]string

	// Key: ExecutorID
	ShardAssignments map[string]AssignedState

	// Key: ShardID
	DrainedShards map[string]struct{}

	// Revision is the store revision the snapshot was read at. Readers that cache the
	// state use it to discard a snapshot older than the one they hold.
	Revision int64
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

// Executor returns the view of an executor.
// It reports false if the executor has no heartbeat.
func (ns *NamespaceState) Executor(executorID string) (Executor, bool) {
	heartbeat, ok := ns.Executors[executorID]
	if !ok {
		return Executor{}, false
	}
	return ns.newExecutor(executorID, heartbeat), true
}

// AssignableExecutorIDs returns the IDs of the executors that may hold shards
func (ns *NamespaceState) AssignableExecutorIDs(staleExecutors map[string]int64) []string {
	var ids []string
	for executorID, heartbeat := range ns.Executors {
		if ns.newExecutor(executorID, heartbeat).IsAssignable(staleExecutors) {
			ids = append(ids, executorID)
		}
	}
	slices.Sort(ids)
	return ids
}

func (ns *NamespaceState) newExecutor(executorID string, heartbeat HeartbeatState) Executor {
	hostname := heartbeat.Hostname()
	return Executor{
		ID:          executorID,
		Heartbeat:   heartbeat,
		HostDrained: hostname != "" && ns.IsHostDrained(hostname),
	}
}
