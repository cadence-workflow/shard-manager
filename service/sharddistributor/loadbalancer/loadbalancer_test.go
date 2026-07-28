package loadbalancer

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/loadbalancer/plan"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

func TestPlanInitialPlacement(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		wantErr bool
	}{
		{name: "naive", mode: config.LoadBalancingModeNAIVE},
		{name: "greedy", mode: config.LoadBalancingModeGREEDY},
		{name: "invalid", mode: config.LoadBalancingModeINVALID, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				LoadBalancingMode: func(namespace string) string {
					return tt.mode
				},
			}
			placements, err := PlanInitialPlacement(cfg, "test-namespace", &store.NamespaceState{}, nil)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, placements)
				return
			}
			require.NoError(t, err)
			assert.Empty(t, placements)
		})
	}
}

func TestPlanInitialPlacement_NoActiveExecutors(t *testing.T) {
	cfg := &config.Config{
		LoadBalancingMode: func(namespace string) string {
			return config.LoadBalancingModeNAIVE
		},
	}

	_, err := PlanInitialPlacement(cfg, "test-namespace", &store.NamespaceState{}, []string{"shard-1"})
	assert.True(t, errors.Is(err, plan.ErrNoActiveExecutors))
}

func TestPlanInitialPlacement_DeduplicatesShardIDs(t *testing.T) {
	tests := []struct {
		name     string
		shardIDs []string
		want     []string
	}{
		{
			name:     "repeated shard collapses to a single placement",
			shardIDs: []string{"shard-1", "shard-1", "shard-1"},
			want:     []string{"shard-1"},
		},
		{
			name:     "duplicates removed preserving first-seen order",
			shardIDs: []string{"shard-b", "shard-a", "shard-b", "shard-c", "shard-a"},
			want:     []string{"shard-b", "shard-a", "shard-c"},
		},
		{
			name:     "input without duplicates is unchanged",
			shardIDs: []string{"shard-a", "shard-b"},
			want:     []string{"shard-a", "shard-b"},
		},
	}

	cfg := &config.Config{
		LoadBalancingMode: func(namespace string) string {
			return config.LoadBalancingModeNAIVE
		},
	}
	state := &store.NamespaceState{
		Executors: map[string]store.HeartbeatState{
			"exec-1": {Status: types.ExecutorStatusACTIVE},
			"exec-2": {Status: types.ExecutorStatusACTIVE},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			placements, err := PlanInitialPlacement(cfg, "test-namespace", state, tt.shardIDs)
			require.NoError(t, err)

			got := make([]string, 0, len(placements))
			for _, placement := range placements {
				got = append(got, placement.ShardID)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

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
