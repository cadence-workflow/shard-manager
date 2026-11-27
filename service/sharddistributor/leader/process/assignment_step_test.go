package process

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

func TestNextAssignmentStep(t *testing.T) {
	tests := []struct {
		name                string
		storedAssignments   map[string][]string
		plannedAssignments  map[string][]string
		assignableExecutors map[string]struct{}
		staleExecutors      map[string]struct{}
		maxChangedExecutors int

		expectedAssignments       map[string][]string
		expectedExecutorsToDelete []string
		expectedDeferredShards    int
	}{
		{
			name: "changes that fit are written, new executors get an assignment and empty stale executors are deleted",
			storedAssignments: map[string][]string{
				"exec-a":     {"shard-1"},
				"exec-stale": {"shard-2"},
			},
			plannedAssignments: map[string][]string{
				"exec-a": {"shard-1", "shard-2"},
			},
			assignableExecutors: map[string]struct{}{"exec-a": {}, "exec-new": {}},
			staleExecutors:      map[string]struct{}{"exec-stale": {}},
			maxChangedExecutors: 3,

			expectedAssignments: map[string][]string{
				"exec-a":   {"shard-1", "shard-2"},
				"exec-new": nil,
			},
			expectedExecutorsToDelete: []string{"exec-stale"},
		},
		{
			// Only the move of shard-2 fits, and no assignable executor serves it.
			name: "shards that no assignable executor serves move first",
			storedAssignments: map[string][]string{
				"exec-a":        {"shard-1"},
				"exec-draining": {"shard-2"},
			},
			plannedAssignments: map[string][]string{
				"exec-a": nil,
				"exec-b": {"shard-1"},
				"exec-c": {"shard-2"},
			},
			assignableExecutors: map[string]struct{}{"exec-a": {}, "exec-b": {}, "exec-c": {}},
			maxChangedExecutors: 2,

			expectedAssignments: map[string][]string{
				"exec-a":        {"shard-1"},
				"exec-c":        {"shard-2"},
				"exec-draining": nil,
			},
			expectedDeferredShards: 1,
		},
		{
			// The move touches two executors, so storage stays as it is.
			name: "a move is deferred when it needs more executors than the limit",
			storedAssignments: map[string][]string{
				"exec-a": {"shard-1"},
				"exec-b": nil,
			},
			plannedAssignments: map[string][]string{
				"exec-a": nil,
				"exec-b": {"shard-1"},
			},
			assignableExecutors: map[string]struct{}{"exec-a": {}, "exec-b": {}},
			maxChangedExecutors: 1,

			expectedAssignments: map[string][]string{
				"exec-a": {"shard-1"},
				"exec-b": nil,
			},
			expectedDeferredShards: 1,
		},
		{
			// The move takes the whole budget, so exec-new is not written yet.
			name: "a new executor without shards waits for budget left after the shard changes",
			storedAssignments: map[string][]string{
				"exec-a": {"shard-1"},
				"exec-b": nil,
			},
			plannedAssignments: map[string][]string{
				"exec-a": nil,
				"exec-b": {"shard-1"},
			},
			assignableExecutors: map[string]struct{}{"exec-a": {}, "exec-b": {}, "exec-new": {}},
			maxChangedExecutors: 2,

			expectedAssignments: map[string][]string{
				"exec-a": nil,
				"exec-b": {"shard-1"},
			},
		},
		{
			name: "a stale executor keeps the shards that do not fit",
			storedAssignments: map[string][]string{
				"exec-a":     nil,
				"exec-b":     nil,
				"exec-stale": {"shard-1", "shard-2"},
			},
			plannedAssignments: map[string][]string{
				"exec-a": {"shard-1"},
				"exec-b": {"shard-2"},
			},
			assignableExecutors: map[string]struct{}{"exec-a": {}, "exec-b": {}},
			staleExecutors:      map[string]struct{}{"exec-stale": {}},
			maxChangedExecutors: 2,

			expectedAssignments: map[string][]string{
				"exec-a":     {"shard-1"},
				"exec-b":     nil,
				"exec-stale": {"shard-2"},
			},
			expectedDeferredShards: 1,
		},
		{
			name: "a stale executor is deleted in the round that empties it",
			storedAssignments: map[string][]string{
				"exec-a":     nil,
				"exec-c":     {"shard-2"},
				"exec-d":     nil,
				"exec-stale": {"shard-1"},
			},
			plannedAssignments: map[string][]string{
				"exec-a": {"shard-1"},
				"exec-c": nil,
				"exec-d": {"shard-2"},
			},
			assignableExecutors: map[string]struct{}{"exec-a": {}, "exec-c": {}, "exec-d": {}},
			staleExecutors:      map[string]struct{}{"exec-stale": {}},
			maxChangedExecutors: 2,

			expectedAssignments: map[string][]string{
				"exec-a": {"shard-1"},
				"exec-c": {"shard-2"},
				"exec-d": nil,
			},
			expectedExecutorsToDelete: []string{"exec-stale"},
			expectedDeferredShards:    1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			storedAssignments := storedShards(tc.storedAssignments)

			assignmentStep := nextAssignmentStep(
				storedAssignments,
				tc.plannedAssignments,
				tc.assignableExecutors,
				tc.staleExecutors,
				tc.maxChangedExecutors,
			)

			// The step returns each executor's shards in no particular order.
			for _, shardIDs := range assignmentStep.assignments {
				slices.Sort(shardIDs)
			}
			assert.Equal(t, tc.expectedAssignments, assignmentStep.assignments)
			assert.ElementsMatch(t, tc.expectedExecutorsToDelete, assignmentStep.executorsToDelete)
			assert.Equal(t, tc.expectedDeferredShards, assignmentStep.deferredShards)
		})
	}
}

func storedShards(shardsByExecutor map[string][]string) map[string]store.AssignedState {
	storedAssignments := make(map[string]store.AssignedState, len(shardsByExecutor))
	for executorID, shardIDs := range shardsByExecutor {
		assigned := make(map[string]*types.ShardAssignment, len(shardIDs))
		for _, shardID := range shardIDs {
			assigned[shardID] = &types.ShardAssignment{Status: types.AssignmentStatusREADY}
		}
		storedAssignments[executorID] = store.AssignedState{AssignedShards: assigned}
	}
	return storedAssignments
}
