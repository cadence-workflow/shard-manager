package greedy

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

func TestPrepareShardStatistics(t *testing.T) {
	now := time.Now().UTC()
	previousUpdateTime := now.Add(-time.Minute)
	previousMoveTime := now.Add(-time.Hour)

	updatedShardID := "updated-shard"
	previousUpdatedShardLoad := 10.0
	updatedShardLoad := 30.0

	preservedShardID := "preserved-shard"
	preservedShardLoad := 20.0

	unassignedShardID := "unassigned-shard"
	unassignedShardLoad := 1000.0

	emptyReportShardID := "empty-report-shard"

	assignedState := &store.AssignedState{
		AssignedShards: map[string]*types.ShardAssignment{
			updatedShardID:     {Status: types.AssignmentStatusREADY},
			preservedShardID:   {Status: types.AssignmentStatusREADY},
			emptyReportShardID: {Status: types.AssignmentStatusREADY},
		},
	}
	previousStats := map[string]store.ShardStatistics{
		updatedShardID: {
			SmoothedLoad: previousUpdatedShardLoad,
			LastMoveTime: previousMoveTime,
		},
		preservedShardID: {
			SmoothedLoad:   preservedShardLoad,
			LastUpdateTime: previousUpdateTime,
			LastMoveTime:   previousMoveTime,
		},
	}
	reports := map[string]*types.ShardStatusReport{
		updatedShardID:     {ShardLoad: updatedShardLoad},
		unassignedShardID:  {ShardLoad: unassignedShardLoad},
		emptyReportShardID: nil,
	}
	logger, logs := testlogger.NewObserved(t)

	got, shouldWrite := PrepareShardStatistics(
		testGreedyConfig(),
		testNamespace,
		"executor-1",
		reports,
		assignedState,
		previousStats,
		now,
		logger,
	)

	require.True(t, shouldWrite)
	expectedStats := map[string]store.ShardStatistics{
		updatedShardID: {
			SmoothedLoad:   updatedShardLoad,
			LastUpdateTime: now,
			LastMoveTime:   previousMoveTime,
		},
		preservedShardID: previousStats[preservedShardID],
	}
	assert.Equal(t, expectedStats, got)
	entries := logs.FilterMessage("empty report, skipping smoothed load update").All()
	require.Len(t, entries, 1)
	assert.Equal(t, zapcore.WarnLevel, entries[0].Level)
}

func TestPrepareShardStatisticsReturnsNoUpdateWithoutEligibleReports(t *testing.T) {
	assignedShardID := "assigned-shard"

	unassignedShardID := "unassigned-shard"
	unassignedShardLoad := 100.0

	assignedState := &store.AssignedState{
		AssignedShards: map[string]*types.ShardAssignment{
			assignedShardID: {Status: types.AssignmentStatusREADY},
		},
	}
	reports := map[string]*types.ShardStatusReport{
		unassignedShardID: {ShardLoad: unassignedShardLoad},
	}

	got, shouldWrite := PrepareShardStatistics(
		testGreedyConfig(),
		testNamespace,
		"executor-1",
		reports,
		assignedState,
		nil,
		time.Now().UTC(),
		testlogger.New(t),
	)

	assert.False(t, shouldWrite)
	assert.Empty(t, got)
}

func TestPrepareShardStatisticsSkipsInvalidReport(t *testing.T) {
	invalidReportShardID := "invalid-report-shard"
	invalidReportedLoad := math.NaN()
	previousSmoothedLoad := 10.0

	validReportShardID := "valid-report-shard"
	validReportedLoad := 20.0

	now := time.Now().UTC()
	previousMoveTime := now.Add(-time.Hour)
	assignedState := &store.AssignedState{
		AssignedShards: map[string]*types.ShardAssignment{
			invalidReportShardID: {Status: types.AssignmentStatusREADY},
			validReportShardID:   {Status: types.AssignmentStatusREADY},
		},
	}
	previousStats := map[string]store.ShardStatistics{
		invalidReportShardID: {
			SmoothedLoad: previousSmoothedLoad,
			LastMoveTime: previousMoveTime,
		},
	}
	reports := map[string]*types.ShardStatusReport{
		invalidReportShardID: {ShardLoad: invalidReportedLoad},
		validReportShardID:   {ShardLoad: validReportedLoad},
	}
	logger, logs := testlogger.NewObserved(t)

	got, shouldWrite := PrepareShardStatistics(
		testGreedyConfig(),
		testNamespace,
		"executor-1",
		reports,
		assignedState,
		previousStats,
		now,
		logger,
	)

	require.True(t, shouldWrite)
	assert.Equal(t, previousStats[invalidReportShardID], got[invalidReportShardID])
	assert.Equal(t, validReportedLoad, got[validReportShardID].SmoothedLoad)
	entries := logs.FilterMessage("failed to calculate smoothed load").All()
	require.Len(t, entries, 1)
	assert.Equal(t, zapcore.ErrorLevel, entries[0].Level)
}
