package greedy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

func TestPrepareAssignmentStatistics(t *testing.T) {
	now := time.Date(2026, time.August, 21, 0, 0, 0, 0, time.UTC)

	sourceExecutorID := "source"
	sourceShardID := "stay-source"
	unmeasuredSourceShardID := "stay-source-unmeasured"
	sourceShardStatistics := store.ShardStatistics{
		SmoothedLoad:   20,
		LastUpdateTime: now.Add(-time.Minute),
	}

	destinationExecutorID := "destination"
	destinationShardID := "stay-destination"
	destinationShardStatistics := store.ShardStatistics{
		SmoothedLoad:   10,
		LastUpdateTime: now.Add(-time.Minute),
	}

	movedShardID := "move"
	movedShardStatistics := store.ShardStatistics{
		SmoothedLoad:   5,
		LastUpdateTime: now.Add(-time.Minute),
		LastMoveTime:   now.Add(-time.Hour),
	}
	expectedMovedShardStatistics := movedShardStatistics
	expectedMovedShardStatistics.LastMoveTime = now

	newShardID := "new"

	// Move one shard from the heavier source to the lighter destination, and
	// assign one new shard.
	previousStatistics := map[string]store.ShardStatistics{
		movedShardID:       movedShardStatistics,
		sourceShardID:      sourceShardStatistics,
		destinationShardID: destinationShardStatistics,
	}
	previousOwnersByShard := map[string]string{
		movedShardID:            sourceExecutorID,
		sourceShardID:           sourceExecutorID,
		unmeasuredSourceShardID: sourceExecutorID,
		destinationShardID:      destinationExecutorID,
	}
	newAssignments := map[string]store.AssignedState{
		sourceExecutorID: {
			AssignedShards: map[string]*types.ShardAssignment{
				sourceShardID:           {Status: types.AssignmentStatusREADY},
				unmeasuredSourceShardID: {Status: types.AssignmentStatusREADY},
			},
		},
		destinationExecutorID: {
			AssignedShards: map[string]*types.ShardAssignment{
				movedShardID:       {Status: types.AssignmentStatusREADY},
				destinationShardID: {Status: types.AssignmentStatusREADY},
				newShardID:         {Status: types.AssignmentStatusREADY},
			},
		},
	}

	updates := PrepareAssignmentStatistics(
		previousOwnersByShard,
		newAssignments,
		previousStatistics,
		now,
	)

	updatesByExecutor := make(map[string]map[string]store.ShardStatistics, len(updates))
	for _, update := range updates {
		updatesByExecutor[update.ExecutorID] = update.Statistics
	}

	// The moved shard carries its statistics to the destination. Existing shards
	// preserve their statistics or remain unmeasured, while the new shard starts empty.
	expectedUpdates := map[string]map[string]store.ShardStatistics{
		sourceExecutorID: {
			sourceShardID:           sourceShardStatistics,
			unmeasuredSourceShardID: {},
		},
		destinationExecutorID: {
			movedShardID:       expectedMovedShardStatistics,
			destinationShardID: destinationShardStatistics,
			newShardID:         {},
		},
	}
	assert.Equal(t, expectedUpdates, updatesByExecutor)
}
