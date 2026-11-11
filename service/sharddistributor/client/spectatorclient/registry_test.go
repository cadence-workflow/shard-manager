package spectatorclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/types"
)

func TestRegistryLookup(t *testing.T) {
	r := newRegistry()
	r.applyPolled(&types.WatchNamespaceStateResponse{
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
	r.applyPolled(&types.WatchNamespaceStateResponse{
		Executors:        []*types.ExecutorShardAssignment{{ExecutorID: "executor-1", AssignedShards: []*types.Shard{{ShardKey: "shard-1"}}}},
		DrainedShardKeys: []string{"shard-2"},
	})
	r.applyPolled(&types.WatchNamespaceStateResponse{})

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

func TestRegistryRefusedStateReleasesWaiters(t *testing.T) {
	r := newRegistry()
	r.applyStreamed(&types.WatchNamespaceStateResponse{AssignmentRevision: 10})
	r.resetReady() // disabled and re-enabled
	r.applyStreamed(&types.WatchNamespaceStateResponse{AssignmentRevision: 9})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, err := r.lookup(ctx, "shard-1")
	assert.NoError(t, err, "the held state is newer, so lookups must not block")
}

func TestRegistryRefusesOlderState(t *testing.T) {
	r := newRegistry()
	assert.True(t, r.applyPolled(&types.WatchNamespaceStateResponse{AssignmentRevision: 10}))
	assert.False(t, r.applyPolled(&types.WatchNamespaceStateResponse{AssignmentRevision: 9}), "a host with an older cache must not roll the state back")
	assert.True(t, r.applyPolled(&types.WatchNamespaceStateResponse{}), "an older server sends no revision and is always applied")
}

func TestRegistryStreamBehind(t *testing.T) {
	tests := []struct {
		name         string
		delivered    []int64
		wantStreamed int64
		wantBehind   bool
	}{
		{name: "nothing delivered yet", wantStreamed: -1, wantBehind: true},
		{name: "behind", delivered: []int64{3}, wantStreamed: 3, wantBehind: true},
		{name: "caught up", delivered: []int64{5}, wantStreamed: 5},
		{name: "refused state still counts as delivered", delivered: []int64{6, 4}, wantStreamed: 4, wantBehind: true},
		{name: "older server sends no revision", delivered: []int64{0}, wantStreamed: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRegistry()
			r.resetStreamed()
			for _, revision := range tt.delivered {
				r.applyStreamed(&types.WatchNamespaceStateResponse{AssignmentRevision: revision})
			}
			streamed, behind := r.streamBehind(5)
			assert.Equal(t, tt.wantStreamed, streamed)
			assert.Equal(t, tt.wantBehind, behind)
		})
	}
}
