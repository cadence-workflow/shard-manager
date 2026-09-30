package handler

import (
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

func toTypesNamespaceExecutorState(executorID string, heartbeat store.HeartbeatState, assigned *store.AssignedState) *types.NamespaceExecutorState {
	return &types.NamespaceExecutorState{
		ExecutorID:     executorID,
		Status:         heartbeat.Status,
		LastHeartbeat:  heartbeat.LastHeartbeat,
		Metadata:       heartbeat.Metadata,
		AssignedShards: toTypesExecutorAssignedShards(assigned),
		HostMetadata:   heartbeat.HostMetadata,
	}
}

func toTypesExecutorAssignedShards(assigned *store.AssignedState) []*types.ExecutorAssignedShardState {
	if assigned == nil {
		return []*types.ExecutorAssignedShardState{}
	}
	shards := make([]*types.ExecutorAssignedShardState, 0, len(assigned.AssignedShards))
	for shardKey, shardAssignment := range assigned.AssignedShards {
		status := types.AssignmentStatusINVALID
		if shardAssignment != nil {
			status = shardAssignment.Status
		}
		shards = append(shards, &types.ExecutorAssignedShardState{
			ShardKey:                 shardKey,
			AssignmentStatus:         status,
			AssignedStateModRevision: assigned.ModRevision,
		})
	}
	return shards
}

func toTypesHeartbeatStates(heartbeats map[string]store.HeartbeatState) map[string]*types.HeartbeatState {
	out := make(map[string]*types.HeartbeatState, len(heartbeats))
	for executorID, heartbeat := range heartbeats {
		out[executorID] = &types.HeartbeatState{
			LastHeartbeat:  heartbeat.LastHeartbeat,
			Status:         heartbeat.Status,
			ReportedShards: heartbeat.ReportedShards,
			Metadata:       heartbeat.Metadata,
		}
	}
	return out
}

func toTypesShardStatistics(stats map[string]store.ShardStatistics) map[string]*types.ShardStatistics {
	out := make(map[string]*types.ShardStatistics, len(stats))
	for shardKey, statistics := range stats {
		out[shardKey] = &types.ShardStatistics{
			SmoothedLoad:   statistics.SmoothedLoad,
			LastUpdateTime: statistics.LastUpdateTime,
			LastMoveTime:   statistics.LastMoveTime,
		}
	}
	return out
}

func toTypesAssignedStates(assignments map[string]store.AssignedState) map[string]*types.AssignedState {
	out := make(map[string]*types.AssignedState, len(assignments))
	for executorID, assignment := range assignments {
		out[executorID] = &types.AssignedState{
			AssignedShards:     assignment.AssignedShards,
			ShardHandoverStats: toTypesShardHandoverStats(assignment.ShardHandoverStats),
			LastUpdated:        assignment.LastUpdated,
			ModRevision:        assignment.ModRevision,
		}
	}
	return out
}

func toTypesShardHandoverStats(stats map[string]store.ShardHandoverStats) map[string]*types.ShardHandoverStats {
	out := make(map[string]*types.ShardHandoverStats, len(stats))
	for shardKey, statistics := range stats {
		out[shardKey] = &types.ShardHandoverStats{
			PreviousExecutorLastHeartbeatTime: statistics.PreviousExecutorLastHeartbeatTime,
			HandoverType:                      statistics.HandoverType,
		}
	}
	return out
}

func toTypesDrainedHost(host store.DrainedHost) *types.DrainedHost {
	return &types.DrainedHost{
		Hostname:  host.Hostname,
		DrainedAt: host.DrainedAt,
		DrainedBy: host.DrainedBy,
		Reason:    host.Reason,
	}
}

func fromTypesDrainedHost(host *types.DrainedHost) store.DrainedHost {
	return store.DrainedHost{
		Hostname:  host.GetHostname(),
		DrainedAt: host.GetDrainedAt(),
		DrainedBy: host.GetDrainedBy(),
		Reason:    host.GetReason(),
	}
}

func toTypesDrainedHosts(hosts []store.DrainedHost) []*types.DrainedHost {
	var out []*types.DrainedHost
	for _, host := range hosts {
		out = append(out, toTypesDrainedHost(host))
	}
	return out
}

func fromTypesDrainedHosts(hosts []*types.DrainedHost) []store.DrainedHost {
	out := make([]store.DrainedHost, 0, len(hosts))
	for _, host := range hosts {
		out = append(out, fromTypesDrainedHost(host))
	}
	return out
}

func toTypesDrainedHostsByName(hosts map[string]store.DrainedHost) map[string]*types.DrainedHost {
	out := make(map[string]*types.DrainedHost, len(hosts))
	for name, host := range hosts {
		out[name] = toTypesDrainedHost(host)
	}
	return out
}

func toTypesExecutorShardAssignments(executorToShards map[*store.ShardOwner][]string) []*types.ExecutorShardAssignment {
	out := make([]*types.ExecutorShardAssignment, 0, len(executorToShards))
	for owner, shardIDs := range executorToShards {
		out = append(out, &types.ExecutorShardAssignment{
			ExecutorID:     owner.ExecutorID,
			AssignedShards: toTypesShards(shardIDs),
			Metadata:       owner.Metadata,
		})
	}
	return out
}

func toTypesShards(shardIDs []string) []*types.Shard {
	shards := make([]*types.Shard, 0, len(shardIDs))
	for _, shardID := range shardIDs {
		shards = append(shards, &types.Shard{ShardKey: shardID})
	}
	return shards
}
