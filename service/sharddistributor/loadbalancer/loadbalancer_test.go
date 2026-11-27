package loadbalancer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

func TestPlanRebalance(t *testing.T) {
	cfg := &config.Config{
		LoadBalancingMode: func(namespace string) string {
			return config.LoadBalancingModeINVALID
		},
	}
	moves, err := PlanRebalance(cfg, "test-namespace", &store.NamespaceState{}, nil, time.Time{}, nil, metrics.NoopScope)
	require.Error(t, err)
	assert.Nil(t, moves)
	assert.ErrorContains(t, err, "unsupported load balancing mode")
}

func TestPrepareShardStatisticsSkipsNaiveMode(t *testing.T) {
	cfg := &config.Config{
		LoadBalancingMode: func(namespace string) string {
			return config.LoadBalancingModeNAIVE
		},
	}
	assignedState := &store.AssignedState{
		AssignedShards: map[string]*types.ShardAssignment{
			"shard-1": {Status: types.AssignmentStatusREADY},
		},
	}
	reports := map[string]*types.ShardStatusReport{
		"shard-1": {ShardLoad: 12.3},
	}

	got, shouldWrite, err := PrepareShardStatistics(
		cfg,
		"test-namespace",
		"executor-1",
		reports,
		assignedState,
		nil,
		time.Now().UTC(),
		log.NewNoop(),
	)

	require.NoError(t, err)
	assert.False(t, shouldWrite)
	assert.Nil(t, got)
}

func TestPrepareAssignmentStatisticsDispatch(t *testing.T) {
	tests := []struct {
		name            string
		mode            string
		expectedUpdates []store.ExecutorShardStatistics
		expectError     bool
	}{
		{
			name: "naive mode skips assignment statistics",
			mode: config.LoadBalancingModeNAIVE,
		},
		{
			name: "greedy mode initializes statistics for a new shard",
			mode: config.LoadBalancingModeGREEDY,
			expectedUpdates: []store.ExecutorShardStatistics{
				{
					ExecutorID: "executor-1",
					Statistics: map[string]store.ShardStatistics{
						"shard-1": {},
					},
				},
			},
		},
		{
			name:        "unsupported mode returns an error",
			mode:        config.LoadBalancingModeINVALID,
			expectError: true,
		},
	}
	// With no previous assignment or statistics, greedy treats shard-1 as newly
	// assigned and records a zero value statistic for executor-1.
	newAssignments := map[string]store.AssignedState{
		"executor-1": {
			AssignedShards: map[string]*types.ShardAssignment{
				"shard-1": {Status: types.AssignmentStatusREADY},
			},
		},
	}
	now := time.Now().UTC()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				LoadBalancingMode: func(string) string { return tt.mode },
			}

			updates, err := PrepareAssignmentStatistics(
				cfg,
				"test-namespace",
				nil,
				newAssignments,
				nil,
				now,
			)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, updates)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedUpdates, updates)
		})
	}
}
