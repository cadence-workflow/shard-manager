package store

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cadence-workflow/shard-manager/common/types"
)

func TestNamespaceState_CountExecutorsByStatus(t *testing.T) {
	tests := []struct {
		name      string
		executors map[string]HeartbeatState
		expected  map[types.ExecutorStatus]int
	}{
		{
			name:      "empty executors",
			executors: map[string]HeartbeatState{},
			expected:  map[types.ExecutorStatus]int{},
		},
		{
			name: "single active executor",
			executors: map[string]HeartbeatState{
				"exec-1": {Status: types.ExecutorStatusACTIVE},
			},
			expected: map[types.ExecutorStatus]int{
				types.ExecutorStatusACTIVE: 1,
			},
		},
		{
			name: "multiple executors same status",
			executors: map[string]HeartbeatState{
				"exec-1": {Status: types.ExecutorStatusACTIVE},
				"exec-2": {Status: types.ExecutorStatusACTIVE},
				"exec-3": {Status: types.ExecutorStatusACTIVE},
			},
			expected: map[types.ExecutorStatus]int{
				types.ExecutorStatusACTIVE: 3,
			},
		},
		{
			name: "all statuses",
			executors: map[string]HeartbeatState{
				"exec-1": {Status: types.ExecutorStatusINVALID},
				"exec-2": {Status: types.ExecutorStatusACTIVE},
				"exec-3": {Status: types.ExecutorStatusDRAINING},
				"exec-4": {Status: types.ExecutorStatusDRAINED},
			},
			expected: map[types.ExecutorStatus]int{
				types.ExecutorStatusINVALID:  1,
				types.ExecutorStatusACTIVE:   1,
				types.ExecutorStatusDRAINING: 1,
				types.ExecutorStatusDRAINED:  1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := &NamespaceState{
				Executors: tt.executors,
			}
			result := ns.CountExecutorsByStatus()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNamespaceState_ShardOwners(t *testing.T) {
	ready := func(shards ...string) AssignedState {
		assigned := make(map[string]*types.ShardAssignment, len(shards))
		for _, shard := range shards {
			assigned[shard] = &types.ShardAssignment{Status: types.AssignmentStatusREADY}
		}
		return AssignedState{AssignedShards: assigned}
	}

	tests := []struct {
		name             string
		executors        map[string]HeartbeatState
		shardAssignments map[string]AssignedState
		expected         map[string]string
	}{
		{
			name:     "empty state",
			expected: map[string]string{},
		},
		{
			name: "all assignments are flattened",
			executors: map[string]HeartbeatState{
				"exec-1": {Status: types.ExecutorStatusACTIVE},
				"exec-2": {Status: types.ExecutorStatusACTIVE},
			},
			shardAssignments: map[string]AssignedState{
				"exec-1": ready("shard-1", "shard-2"),
				"exec-2": ready("shard-3"),
			},
			expected: map[string]string{
				"shard-1": "exec-1",
				"shard-2": "exec-1",
				"shard-3": "exec-2",
			},
		},
		{
			name: "shards of draining and drained executors are included",
			executors: map[string]HeartbeatState{
				"exec-active":   {Status: types.ExecutorStatusACTIVE},
				"exec-draining": {Status: types.ExecutorStatusDRAINING},
				"exec-drained":  {Status: types.ExecutorStatusDRAINED},
			},
			shardAssignments: map[string]AssignedState{
				"exec-active":   ready("shard-1"),
				"exec-draining": ready("shard-2"),
				"exec-drained":  ready("shard-3"),
			},
			expected: map[string]string{
				"shard-1": "exec-active",
				"shard-2": "exec-draining",
				"shard-3": "exec-drained",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := &NamespaceState{
				Executors:        tt.executors,
				ShardAssignments: tt.shardAssignments,
			}
			assert.Equal(t, tt.expected, ns.ShardOwners())
		})
	}
}
