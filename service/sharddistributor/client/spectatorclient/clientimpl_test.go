package spectatorclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/clientcommon"
)

func TestWatchLoopBasicFlow(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctrl := gomock.NewController(t)
	mockClient := sharddistributor.NewMockClient(ctrl)
	mockStream := sharddistributor.NewMockWatchNamespaceStateClient(ctrl)

	// Create a context to control when the mock stream should unblock
	streamCtx, cancelStream := context.WithCancel(context.Background())

	spectator := newTestSpectator(t, Params{Client: mockClient})

	// Expect stream creation
	mockClient.EXPECT().
		WatchNamespaceState(gomock.Any(), &types.WatchNamespaceStateRequest{Namespace: "test-ns"}).
		Return(mockStream, nil)

	// First Recv returns state
	mockStream.EXPECT().Recv().Return(&types.WatchNamespaceStateResponse{
		Executors: []*types.ExecutorShardAssignment{
			{
				ExecutorID: "executor-1",
				Metadata: map[string]string{
					"grpc_address": "127.0.0.1:7953",
				},
				AssignedShards: []*types.Shard{
					{ShardKey: "shard-1"},
					{ShardKey: "shard-2"},
				},
			},
		},
	}, nil)

	// Second Recv blocks until shutdown
	mockStream.EXPECT().Recv().DoAndReturn(func(...interface{}) (*types.WatchNamespaceStateResponse, error) {
		// Wait for context to be done
		<-streamCtx.Done()
		return nil, streamCtx.Err()
	})

	ctx := context.Background()
	err := spectator.Start(ctx)
	require.NoError(t, err)
	defer func() {
		cancelStream()
		spectator.Stop()
	}()

	// Query shard owner
	owner, err := spectator.GetShardOwner(context.Background(), "shard-1")
	assert.NoError(t, err)
	assert.Equal(t, "executor-1", owner.ExecutorID)
	assert.Equal(t, "127.0.0.1:7953", owner.Metadata["grpc_address"])

	owner, err = spectator.GetShardOwner(context.Background(), "shard-2")
	assert.NoError(t, err)
	assert.Equal(t, "executor-1", owner.ExecutorID)
}

func TestGetShardOwner_CacheMiss_FallbackToRPC(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctrl := gomock.NewController(t)
	mockClient := sharddistributor.NewMockClient(ctrl)
	mockStream := sharddistributor.NewMockWatchNamespaceStateClient(ctrl)

	// Create a context to control when the mock stream should unblock
	streamCtx, cancelStream := context.WithCancel(context.Background())

	spectator := newTestSpectator(t, Params{Client: mockClient})

	// Setup stream
	mockClient.EXPECT().
		WatchNamespaceState(gomock.Any(), gomock.Any()).
		Return(mockStream, nil)

	// First Recv returns state
	mockStream.EXPECT().Recv().Return(&types.WatchNamespaceStateResponse{
		Executors: []*types.ExecutorShardAssignment{
			{
				ExecutorID: "executor-1",
				Metadata: map[string]string{
					"grpc_address": "127.0.0.1:7953",
				},
				AssignedShards: []*types.Shard{{ShardKey: "shard-1"}},
			},
		},
	}, nil)

	// Second Recv blocks until shutdown
	mockStream.EXPECT().Recv().AnyTimes().DoAndReturn(func(...interface{}) (*types.WatchNamespaceStateResponse, error) {
		// Wait for context to be done
		<-streamCtx.Done()
		return nil, streamCtx.Err()
	})

	// Expect RPC fallback for unknown shard
	mockClient.EXPECT().
		GetShardOwner(gomock.Any(), &types.GetShardOwnerRequest{
			Namespace: "test-ns",
			ShardKey:  "unknown-shard",
		}).
		Return(&types.GetShardOwnerResponse{
			Owner: "executor-2",
			Metadata: map[string]string{
				"grpc_address": "127.0.0.1:7954",
			},
		}, nil)

	spectator.Start(context.Background())
	defer func() {
		cancelStream()
		spectator.Stop()
	}()

	// Cache hit
	owner, err := spectator.GetShardOwner(context.Background(), "shard-1")
	assert.NoError(t, err)
	assert.Equal(t, "executor-1", owner.ExecutorID)

	// Cache miss - should trigger RPC
	owner, err = spectator.GetShardOwner(context.Background(), "unknown-shard")
	assert.NoError(t, err)
	assert.Equal(t, "executor-2", owner.ExecutorID)
	assert.Equal(t, "127.0.0.1:7954", owner.Metadata["grpc_address"])
}

func TestGetShardOwner_DrainedShard(t *testing.T) {
	assignedToExecutor1 := []*types.ExecutorShardAssignment{
		{
			ExecutorID:     "executor-1",
			Metadata:       map[string]string{"grpc_address": "127.0.0.1:7953"},
			AssignedShards: []*types.Shard{{ShardKey: "shard-1"}},
		},
	}

	tests := []struct {
		name        string
		response    *types.WatchNamespaceStateResponse
		shardKey    string
		wantDrained bool
		wantOwner   string
	}{
		{
			name: "drained shard without an owner",
			response: &types.WatchNamespaceStateResponse{
				DrainedShardKeys: []string{"shard-9"},
			},
			shardKey:    "shard-9",
			wantDrained: true,
		},
		{
			name: "drain wins over a live assignment",
			response: &types.WatchNamespaceStateResponse{
				Executors:        assignedToExecutor1,
				DrainedShardKeys: []string{"shard-1"},
			},
			shardKey:    "shard-1",
			wantDrained: true,
		},
		{
			name: "assigned shard is unaffected by an unrelated drain",
			response: &types.WatchNamespaceStateResponse{
				Executors:        assignedToExecutor1,
				DrainedShardKeys: []string{"shard-9"},
			},
			shardKey:  "shard-1",
			wantOwner: "executor-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			spectator := newTestSpectator(t, Params{Client: sharddistributor.NewMockClient(ctrl)})

			spectator.watcher.handleResponse(tt.response)

			owner, err := spectator.GetShardOwner(context.Background(), tt.shardKey)

			if tt.wantDrained {
				var drainedErr *types.ShardDrainedError
				require.ErrorAs(t, err, &drainedErr)
				assert.Equal(t, "test-ns", drainedErr.Namespace)
				assert.Equal(t, tt.shardKey, drainedErr.ShardKey)
				assert.Nil(t, owner)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOwner, owner.ExecutorID)
		})
	}
}

func TestGetShardOwner_UndrainStopsRefusing(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := sharddistributor.NewMockClient(ctrl)

	spectator := newTestSpectator(t, Params{Client: mockClient})

	spectator.watcher.handleResponse(&types.WatchNamespaceStateResponse{
		DrainedShardKeys: []string{"shard-1"},
	})

	_, err := spectator.GetShardOwner(context.Background(), "shard-1")
	var drainedErr *types.ShardDrainedError
	require.ErrorAs(t, err, &drainedErr)

	spectator.watcher.handleResponse(&types.WatchNamespaceStateResponse{})

	mockClient.EXPECT().
		GetShardOwner(gomock.Any(), &types.GetShardOwnerRequest{Namespace: "test-ns", ShardKey: "shard-1"}).
		Return(&types.GetShardOwnerResponse{Owner: "executor-1"}, nil)

	owner, err := spectator.GetShardOwner(context.Background(), "shard-1")
	require.NoError(t, err)
	assert.Equal(t, "executor-1", owner.ExecutorID)
}

func TestGetShardOwner_TimeoutBeforeFirstState(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctrl := gomock.NewController(t)
	mockClient := sharddistributor.NewMockClient(ctrl)

	spectator := newTestSpectator(t, Params{Client: mockClient})

	// Create a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Try to get shard owner before first state is received
	// Should timeout and return an error
	_, err := spectator.GetShardOwner(ctx, "shard-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "wait for first state")
}

func TestWatchLoopDisabled(t *testing.T) {
	defer goleak.VerifyNone(t)

	timeSource := clock.NewMockedTimeSource()
	spectator := newTestSpectator(t, Params{TimeSource: timeSource, Enabled: func() bool { return false }})

	err := spectator.Start(context.Background())
	assert.NoError(t, err)

	// Disabled state enters a sleep loop, verify it sleeps periodically. The poller waits on its timer too.
	timeSource.BlockUntil(2)
	timeSource.Advance(1200 * time.Millisecond)

	timeSource.BlockUntil(2)
	timeSource.Advance(1200 * time.Millisecond)

	// Stop exits cleanly and calls Done() on the signal
	spectator.Stop()

	// After Stop(), waiters are released
	_, _, err = spectator.registry.lookup(context.Background(), "shard-1")
	assert.NoError(t, err)
}

func newTestSpectator(t *testing.T, params Params) *spectatorImpl {
	params.Logger = zap.NewNop()
	if params.MetricsScope == nil {
		params.MetricsScope = tally.NoopScope
	}
	if params.TimeSource == nil {
		params.TimeSource = clock.NewRealTimeSource()
	}
	spectator, err := newSpectatorWithConfig(params, &clientcommon.NamespaceConfig{Namespace: "test-ns"})
	require.NoError(t, err)
	return spectator.(*spectatorImpl)
}
