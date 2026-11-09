package spectatorclient

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cadence-workflow/shard-manager/common/types"
)

func TestRegistryLookup(t *testing.T) {
	var r registry
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
			owner, drained := r.lookup(tt.shardKey)
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
	var r registry
	r.apply(&types.WatchNamespaceStateResponse{
		Executors:        []*types.ExecutorShardAssignment{{ExecutorID: "executor-1", AssignedShards: []*types.Shard{{ShardKey: "shard-1"}}}},
		DrainedShardKeys: []string{"shard-2"},
	})
	r.apply(&types.WatchNamespaceStateResponse{})

	owner, _ := r.lookup("shard-1")
	assert.Nil(t, owner, "the assignment is gone")
	_, drained := r.lookup("shard-2")
	assert.False(t, drained, "the drain is gone")
}
