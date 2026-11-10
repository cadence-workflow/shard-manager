package spectatorclient

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/types"
)

func TestRegistryLookup(t *testing.T) {
	r := newRegistry()
	r.apply(&types.WatchNamespaceStateResponse{
		Executors: []*types.ExecutorShardAssignment{{
			ExecutorID:     "executor-1",
			AssignedShards: []*types.Shard{{ShardKey: "shard-1"}, {ShardKey: "shard-2"}},
		}},
		DrainedShardKeys: []string{"shard-2", "shard-3"},
	})

	tests := []struct {
		shardKey    string
		wantOwner   string
		wantDrained bool
	}{
		{shardKey: "shard-1", wantOwner: "executor-1"},
		{shardKey: "shard-2", wantOwner: "executor-1", wantDrained: true},
		{shardKey: "shard-3", wantDrained: true},
		{shardKey: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.shardKey, func(t *testing.T) {
			owner, drained, err := r.lookup(context.Background(), tt.shardKey)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDrained, drained)
			if tt.wantOwner == "" {
				assert.Nil(t, owner)
				return
			}
			assert.Equal(t, tt.wantOwner, owner.ExecutorID)
		})
	}
}

func TestRegistryApplyReplacesState(t *testing.T) {
	r := newRegistry()
	r.apply(&types.WatchNamespaceStateResponse{
		Executors:        []*types.ExecutorShardAssignment{{ExecutorID: "executor-1", AssignedShards: []*types.Shard{{ShardKey: "shard-1"}}}},
		DrainedShardKeys: []string{"shard-2"},
	})
	r.apply(&types.WatchNamespaceStateResponse{})

	owner, _, _ := r.lookup(context.Background(), "shard-1")
	assert.Nil(t, owner, "the assignment is gone")
	_, drained, _ := r.lookup(context.Background(), "shard-2")
	assert.False(t, drained, "the drain is gone")
}

func TestRegistryLookupBeforeFirstState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := newRegistry().lookup(ctx, "shard-1")
	assert.ErrorIs(t, err, context.Canceled)
}
