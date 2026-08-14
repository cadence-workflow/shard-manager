package greedy

import (
	"cmp"
	"maps"
	"slices"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/loadbalancer/plan"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

type executorLoad struct {
	shardCount   int
	smoothedLoad float64
}

// PlanInitialPlacement returns planned placements for a batch of unassigned shards.
func PlanInitialPlacement(state *store.NamespaceState, shardIDs []string) ([]plan.Placement, error) {
	loads, averageShardLoad := executorLoads(state)
	placements := make([]plan.Placement, 0, len(shardIDs))
	for _, shardID := range shardIDs {
		executorID, err := chooseExecutorAndUpdateLoads(loads, averageShardLoad)
		if err != nil {
			return nil, err
		}
		placements = append(placements, plan.Placement{
			ShardID:    shardID,
			ExecutorID: executorID,
		})
	}
	return placements, nil
}

func executorLoads(state *store.NamespaceState) (map[string]executorLoad, float64) {
	activeAssignments := activeExecutorAssignments(state)
	averageMeasured := averageMeasuredShardLoad(activeAssignments, state.ShardStats)
	loads := make(map[string]executorLoad, len(activeAssignments))
	for executorID, shards := range activeAssignments {
		load := executorLoad{shardCount: len(shards)}
		for _, shardID := range shards {
			load.smoothedLoad += effectiveShardLoad(shardID, state.ShardStats, averageMeasured)
		}
		loads[executorID] = load
	}

	return loads, averageMeasured
}

func activeExecutorAssignments(state *store.NamespaceState) map[string][]string {
	assignments := make(map[string][]string)
	for executorID, executorState := range state.Executors {
		if executorState.Status != types.ExecutorStatusACTIVE {
			continue
		}
		shards := make([]string, 0, len(state.ShardAssignments[executorID].AssignedShards))
		for shardID := range state.ShardAssignments[executorID].AssignedShards {
			shards = append(shards, shardID)
		}
		assignments[executorID] = shards
	}
	return assignments
}

func chooseExecutorAndUpdateLoads(loads map[string]executorLoad, averageShardLoad float64) (string, error) {
	if len(loads) == 0 {
		return "", plan.ErrNoActiveExecutors
	}
	chosen := slices.MinFunc(slices.Collect(maps.Keys(loads)), func(a, b string) int {
		la, lb := loads[a], loads[b]
		return cmp.Or(
			cmp.Compare(la.smoothedLoad, lb.smoothedLoad),
			cmp.Compare(la.shardCount, lb.shardCount),
			cmp.Compare(a, b),
		)
	})
	load := loads[chosen]
	load.shardCount++
	load.smoothedLoad += averageShardLoad
	loads[chosen] = load
	return chosen, nil
}
