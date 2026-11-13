package testdata

import (
	"time"

	"github.com/cadence-workflow/shard-manager/common/types"
)

var (
	ShardDistributorGetShardOwnerRequest = types.GetShardOwnerRequest{
		ShardKey:  "shard-key",
		Namespace: "namespace",
	}
	ShardDistributorGetShardOwnerResponse = types.GetShardOwnerResponse{
		Owner:     "owner",
		Namespace: "namespace",
		Metadata:  map[string]string{"key-1": "value-1", "key-2": "value-2"},
	}
	ShardDistributorExecutorHeartbeatRequest = types.ExecutorHeartbeatRequest{
		Namespace:  "namespace",
		ExecutorID: "executor-id",
		Status:     types.ExecutorStatusACTIVE,
		ShardStatusReports: map[string]*types.ShardStatusReport{
			"shard-key-1": {
				Status:    types.ShardStatusREADY,
				ShardLoad: 0.5,
			},
			"shard-key-2": {
				Status:    types.ShardStatusINVALID,
				ShardLoad: 0.75,
			},
		},
		RangeLoadReports: map[string]*types.RangeLoadReport{
			"range-1": {
				RangeLoad:    1.5,
				TopTasklists: map[string]float64{"tasklist-1": 0.9, "tasklist-2": 0.6},
			},
		},
		Metadata: map[string]string{
			"key-1": "value-1",
			"key-2": "value-2",
		},
		HostMetadata: &types.HostMetadata{
			HostName: "host-name",
		},
	}
	ShardDistributorExecutorHeartbeatResponse = types.ExecutorHeartbeatResponse{
		ShardAssignments: map[string]*types.ShardAssignment{
			"shard-key-1": {
				Status: types.AssignmentStatusREADY,
			},
			"shard-key-2": {
				Status: types.AssignmentStatusINVALID,
			},
		},
		RangeAssignments: []*types.RangeAssignment{
			rangeAssignment("range-1", 0x00, 0x40),
			finalRangeAssignment("range-2", 0x40),
		},
	}
	ShardDistributorWatchNamespaceStateRequest = types.WatchNamespaceStateRequest{
		Namespace: "namespace",
	}
	ShardDistributorWatchNamespaceStateResponse = types.WatchNamespaceStateResponse{
		Executors: []*types.ExecutorShardAssignment{
			{
				ExecutorID:     "executor-1",
				AssignedShards: []*types.Shard{&types.Shard{ShardKey: "shard-1"}, &types.Shard{ShardKey: "shard-2"}},
				Metadata:       map[string]string{"key-1": "value-1"},
			},
			{
				ExecutorID:     "executor-2",
				AssignedShards: []*types.Shard{&types.Shard{ShardKey: "shard-3"}},
				Metadata:       map[string]string{"key-2": "value-2"},
			},
		},
		DrainedShardKeys:   []string{"shard-4", "shard-5"},
		AssignmentRevision: 5,
		RangeAssignments: []*types.RangeAssignment{
			rangeAssignment("range-1", 0x00, 0x40),
			finalRangeAssignment("range-2", 0x40),
		},
	}
	ShardDistributorGetNamespaceStateIfNewerRequest = types.GetNamespaceStateIfNewerRequest{
		Namespace:          "namespace",
		AssignmentRevision: 4,
	}
	ShardDistributorGetNamespaceStateIfNewerResponse = types.GetNamespaceStateIfNewerResponse{
		AssignmentRevision: 5,
		State:              &ShardDistributorWatchNamespaceStateResponse,
	}
	ShardDistributorGetNamespaceStateRequest = types.GetNamespaceStateRequest{
		Namespace: "namespace",
	}
	ShardDistributorGetNamespaceStateResponse = types.GetNamespaceStateResponse{
		Namespace: "namespace",
		Executors: []*types.NamespaceExecutorState{
			{
				ExecutorID:    "executor-1",
				Status:        types.ExecutorStatusACTIVE,
				LastHeartbeat: time.Date(2024, 1, 2, 3, 4, 5, 6, time.UTC),
				Metadata:      map[string]string{"k": "v"},
				AssignedShards: []*types.ExecutorAssignedShardState{
					{ShardKey: "a", AssignmentStatus: types.AssignmentStatusREADY, AssignedStateModRevision: 7},
				},
				HostMetadata: &types.HostMetadata{HostName: "host-1"},
			},
		},
	}
	ShardDistributorGetFullNamespaceStateRequest = types.GetFullNamespaceStateRequest{
		Namespace: "namespace",
	}
	ShardDistributorGetFullNamespaceStateResponse = types.GetFullNamespaceStateResponse{
		Namespace: "namespace",
		Executors: map[string]*types.HeartbeatState{
			"executor-1": {
				LastHeartbeat: time.Date(2024, 1, 2, 3, 4, 5, 6, time.UTC),
				Status:        types.ExecutorStatusACTIVE,
				ReportedShards: map[string]*types.ShardStatusReport{
					"shard-1": {Status: types.ShardStatusREADY, ShardLoad: 0.5},
				},
				Metadata: map[string]string{"zone": "dca1"},
			},
		},
		ShardStats: map[string]*types.ShardStatistics{
			"shard-1": {
				SmoothedLoad:   0.75,
				LastUpdateTime: time.Date(2024, 1, 2, 3, 4, 6, 0, time.UTC),
				LastMoveTime:   time.Date(2024, 1, 2, 3, 4, 7, 0, time.UTC),
			},
		},
		ShardAssignments: map[string]*types.AssignedState{
			"executor-1": {
				AssignedShards: map[string]*types.ShardAssignment{
					"shard-1": {Status: types.AssignmentStatusREADY},
				},
				ShardHandoverStats: map[string]*types.ShardHandoverStats{
					"shard-1": {
						PreviousExecutorLastHeartbeatTime: time.Date(2024, 1, 2, 3, 4, 3, 0, time.UTC),
						HandoverType:                      types.HandoverTypeGRACEFUL,
					},
				},
				LastUpdated: time.Date(2024, 1, 2, 3, 4, 8, 0, time.UTC),
				ModRevision: 7,
			},
		},
		DrainedShards: []string{"shard-2"},
		DrainedHosts: map[string]*types.DrainedHost{
			"host-1": {
				Hostname:  "host-1",
				DrainedAt: time.Date(2024, 1, 2, 3, 4, 9, 0, time.UTC),
				DrainedBy: "operator",
				Reason:    "maintenance",
			},
		},
		RangeAssignments: []*types.RangeAssignment{
			rangeAssignment("range-1", 0x00, 0x40),
			finalRangeAssignment("range-2", 0x40),
		},
	}
	ShardDistributorGetExecutorStateRequest = types.GetExecutorStateRequest{
		Namespace:  "namespace",
		ExecutorID: "executor-1",
	}
	ShardDistributorGetExecutorStateResponse = types.GetExecutorStateResponse{
		Namespace: "namespace",
		Executor: &types.NamespaceExecutorState{
			ExecutorID:    "executor-1",
			Status:        types.ExecutorStatusACTIVE,
			LastHeartbeat: time.Date(2024, 1, 2, 3, 4, 5, 6, time.UTC),
			Metadata:      map[string]string{"k": "v"},
			AssignedShards: []*types.ExecutorAssignedShardState{
				{ShardKey: "a", AssignmentStatus: types.AssignmentStatusREADY, AssignedStateModRevision: 7},
			},
			HostMetadata: &types.HostMetadata{HostName: "host-1"},
		},
	}
	ShardDistributorListNamespacesRequest  = types.ListNamespacesRequest{}
	ShardDistributorListNamespacesResponse = types.ListNamespacesResponse{
		Namespaces: []*types.NamespaceConfig{
			{
				Name:     "shard-distributor-canary",
				Type:     "fixed",
				Mode:     "onboarded",
				ShardNum: 32,
			},
			{
				Name: "shard-distributor-canary-ephemeral",
				Type: "ephemeral",
				Mode: "distributed_pass",
			},
		},
	}
	ShardDistributorForceResetNamespaceRequest = types.ForceResetNamespaceRequest{
		Namespace: "namespace",
	}
	ShardDistributorForceResetNamespaceResponse = types.ForceResetNamespaceResponse{
		DeletedKeys: 42,
	}
	ShardDistributorDrainHostsRequest = types.DrainHostsRequest{
		Namespace: "namespace",
		Hosts: []*types.DrainedHost{
			{
				Hostname:  "host-a",
				DrainedAt: time.Date(2026, 8, 25, 7, 40, 0, 0, time.UTC),
				DrainedBy: "gaziza",
				Reason:    "maintenance",
			},
		},
	}
	ShardDistributorUndrainHostsRequest = types.UndrainHostsRequest{
		Namespace: "namespace",
		Hostnames: []string{"host-a", "host-b"},
	}
	ShardDistributorUndrainHostsResponse = types.UndrainHostsResponse{
		UndrainedHostnames: []string{"host-a"},
	}
	ShardDistributorGetDrainedHostsRequest = types.GetDrainedHostsRequest{
		Namespace: "namespace",
	}
	ShardDistributorGetDrainedHostsResponse = types.GetDrainedHostsResponse{
		Namespace: "namespace",
		Hosts: []*types.DrainedHost{
			{
				Hostname:  "host-a",
				DrainedAt: time.Date(2026, 8, 25, 7, 40, 0, 0, time.UTC),
				DrainedBy: "gaziza",
				Reason:    "maintenance",
			},
		},
	}
)

func rangeAssignment(id string, low, high byte) *types.RangeAssignment {
	return &types.RangeAssignment{
		ID:         id,
		Range:      types.Range{LowInclusive: b8(low), HighExclusive: b8(high)},
		ExecutorID: "executor-1",
		Status:     types.AssignmentStatusREADY,
	}
}

func finalRangeAssignment(id string, low byte) *types.RangeAssignment {
	return &types.RangeAssignment{
		ID:         id,
		Range:      types.Range{LowInclusive: b8(low)},
		ExecutorID: "executor-2",
		Status:     types.AssignmentStatusREADY,
	}
}

func b8(b byte) []byte {
	return []byte{b, 0, 0, 0, 0, 0, 0, 0}
}
