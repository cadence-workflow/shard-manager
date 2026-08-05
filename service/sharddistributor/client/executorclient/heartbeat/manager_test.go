package heartbeat

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/mock/gomock"
	"go.uber.org/yarpc"

	"github.com/cadence-workflow/shard-manager/client/sharddistributorexecutor"
	"github.com/cadence-workflow/shard-manager/common/types"
)

func newTestManager(t *testing.T, handler func(ctx context.Context, req *types.ExecutorHeartbeatRequest) (*types.ExecutorHeartbeatResponse, error)) *Manager {
	ctrl := gomock.NewController(t)

	mockClient := sharddistributorexecutor.NewMockClient(ctrl)
	mockClient.EXPECT().
		Heartbeat(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req *types.ExecutorHeartbeatRequest, _ ...yarpc.CallOption) (*types.ExecutorHeartbeatResponse, error) {
			return handler(ctx, req)
		}).
		AnyTimes()

	mockState := NewMockStateProvider(ctrl)
	mockState.EXPECT().GetShardStatusReports().Return(nil).AnyTimes()
	mockState.EXPECT().GetMetadata().Return(nil).AnyTimes()

	return NewManager(
		mockClient,
		"test-namespace",
		"test-executor",
		mockState,
		tally.NoopScope,
		time.Second,
	)
}

func asyncHeartbeat(t *testing.T, wg *sync.WaitGroup, m *Manager, shardID string) {
	t.Helper()
	wg.Add(1)
	go func() {
		defer wg.Done()
		result, err := m.Heartbeat(context.Background(), shardID)
		assert.NoError(t, err)
		assert.Contains(t, result, shardID)
	}()
}

func TestManager_SingleCaller(t *testing.T) {
	var rpcCount atomic.Int32
	expected := map[string]*types.ShardAssignment{
		"shard-1": {Status: types.AssignmentStatusREADY},
	}

	m := newTestManager(t, func(_ context.Context, _ *types.ExecutorHeartbeatRequest) (*types.ExecutorHeartbeatResponse, error) {
		rpcCount.Add(1)
		return &types.ExecutorHeartbeatResponse{ShardAssignments: expected}, nil
	})

	result, err := m.Heartbeat(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, expected, result)
	assert.Equal(t, int32(1), rpcCount.Load())
}

func TestManager_RPCError(t *testing.T) {
	var rpcCount atomic.Int32

	m := newTestManager(t, func(_ context.Context, _ *types.ExecutorHeartbeatRequest) (*types.ExecutorHeartbeatResponse, error) {
		rpcCount.Add(1)
		return nil, fmt.Errorf("rpc failed")
	})

	_, err := m.Heartbeat(context.Background(), "")
	require.ErrorContains(t, err, "rpc failed")
	assert.Equal(t, int32(1), rpcCount.Load())
}

func TestManager_ConcurrentCallersCoalesce(t *testing.T) {
	var rpcCount atomic.Int32
	release := make(chan struct{})

	m := newTestManager(t, func(_ context.Context, _ *types.ExecutorHeartbeatRequest) (*types.ExecutorHeartbeatResponse, error) {
		rpcCount.Add(1)
		<-release
		return &types.ExecutorHeartbeatResponse{
			ShardAssignments: map[string]*types.ShardAssignment{
				"shard-1": {Status: types.AssignmentStatusREADY},
			},
		}, nil
	})

	var wg sync.WaitGroup

	for range 10 {
		asyncHeartbeat(t, &wg, m, "shard-1")
	}

	time.Sleep(10 * time.Millisecond)
	close(release)
	wg.Wait()

	assert.Equal(t, int32(1), rpcCount.Load())
}

func TestManager_SharedRetryGetsNewData(t *testing.T) {
	var rpcCount atomic.Int32
	release := make(chan struct{})

	m := newTestManager(t, func(_ context.Context, _ *types.ExecutorHeartbeatRequest) (*types.ExecutorHeartbeatResponse, error) {
		n := rpcCount.Add(1)
		if n == 1 {
			<-release
			return &types.ExecutorHeartbeatResponse{
				ShardAssignments: map[string]*types.ShardAssignment{
					"shard-1": {Status: types.AssignmentStatusREADY},
				},
			}, nil
		}
		return &types.ExecutorHeartbeatResponse{
			ShardAssignments: map[string]*types.ShardAssignment{
				"shard-1": {Status: types.AssignmentStatusREADY},
				"shard-2": {Status: types.AssignmentStatusREADY},
			},
		}, nil
	})

	var wg sync.WaitGroup

	asyncHeartbeat(t, &wg, m, "shard-1")
	asyncHeartbeat(t, &wg, m, "shard-2")

	time.Sleep(10 * time.Millisecond)
	close(release)
	wg.Wait()

	assert.Equal(t, int32(2), rpcCount.Load())
}

func TestManager_DrainingHeartbeat(t *testing.T) {
	var gotStatus types.ExecutorStatus

	m := newTestManager(t, func(_ context.Context, req *types.ExecutorHeartbeatRequest) (*types.ExecutorHeartbeatResponse, error) {
		gotStatus = req.Status
		return &types.ExecutorHeartbeatResponse{}, nil
	})

	err := m.DrainingHeartbeat()
	require.NoError(t, err)
	assert.Equal(t, types.ExecutorStatusDRAINING, gotStatus)
}
