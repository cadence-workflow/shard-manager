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
	"github.com/cadence-workflow/shard-manager/client/wrappers/retryable"
	"github.com/cadence-workflow/shard-manager/client/wrappers/timeout"
	"github.com/cadence-workflow/shard-manager/common"
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
		"test-hostname",
		mockState,
		tally.NoopScope,
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

func TestManager_SendsHostMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)

	var got *types.ExecutorHeartbeatRequest
	mockClient := sharddistributorexecutor.NewMockClient(ctrl)
	mockClient.EXPECT().
		Heartbeat(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *types.ExecutorHeartbeatRequest, _ ...yarpc.CallOption) (*types.ExecutorHeartbeatResponse, error) {
			got = req
			return &types.ExecutorHeartbeatResponse{}, nil
		})

	mockState := NewMockStateProvider(ctrl)
	mockState.EXPECT().GetShardStatusReports().Return(nil)
	mockState.EXPECT().GetMetadata().Return(nil)

	m := NewManager(
		mockClient,
		"test-namespace",
		"test-executor",
		"host-name",
		mockState,
		tally.NoopScope,
		tally.NoopScope,
		time.Second,
	)

	_, err := m.Heartbeat(context.Background(), "")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.HostMetadata)
	assert.Equal(t, "host-name", got.HostMetadata.HostName)
}

func TestManager_Retries(t *testing.T) {
	hang := func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	transient := func(context.Context) error { return &types.InternalServiceError{Message: "boom"} }
	permanent := func(context.Context) error { return &types.BadRequestError{Message: "bad"} }

	for name, tc := range map[string]struct {
		fail         func(ctx context.Context) error
		failures     int
		wantErr      string
		wantAttempts int32
	}{
		"attempt timeout recovers":       {fail: hang, failures: 1, wantAttempts: 2},
		"transient error recovers":       {fail: transient, failures: 1, wantAttempts: 2},
		"permanent error is not retried": {fail: permanent, failures: 1, wantErr: "bad", wantAttempts: 1},
		"budget exhaustion":              {fail: hang, failures: 100, wantErr: "budget"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var attempts atomic.Int32
			m := newTestManager(t, func(ctx context.Context, _ *types.ExecutorHeartbeatRequest) (*types.ExecutorHeartbeatResponse, error) {
				if int(attempts.Add(1)) <= tc.failures {
					return nil, tc.fail(ctx)
				}
				return &types.ExecutorHeartbeatResponse{}, nil
			})

			start := time.Now()
			_, err := m.Heartbeat(context.Background(), "")
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tc.wantAttempts != 0 {
				assert.Equal(t, tc.wantAttempts, attempts.Load())
			} else {
				assert.Less(t, time.Since(start), activeHeartbeatBudget+time.Second)
			}
		})
	}
}

func TestManager_ConcurrentCallersShareRecovery(t *testing.T) {
	var rpcCount atomic.Int32
	release := make(chan struct{})

	m := newTestManager(t, func(ctx context.Context, _ *types.ExecutorHeartbeatRequest) (*types.ExecutorHeartbeatResponse, error) {
		if rpcCount.Add(1) == 1 {
			<-release
			return nil, &types.InternalServiceError{Message: "boom"}
		}
		return &types.ExecutorHeartbeatResponse{ShardAssignments: map[string]*types.ShardAssignment{
			"shard-1": {Status: types.AssignmentStatusREADY},
		}}, nil
	})

	var wg sync.WaitGroup
	for range 10 {
		asyncHeartbeat(t, &wg, m, "shard-1")
	}
	time.Sleep(10 * time.Millisecond)
	close(release)
	wg.Wait()

	assert.Equal(t, int32(2), rpcCount.Load())
}

func TestManager_RecoversAfterFailedFlight(t *testing.T) {
	var rpcCount atomic.Int32
	m := newTestManager(t, func(_ context.Context, _ *types.ExecutorHeartbeatRequest) (*types.ExecutorHeartbeatResponse, error) {
		if rpcCount.Add(1) == 1 {
			return nil, &types.BadRequestError{Message: "bad"}
		}
		return &types.ExecutorHeartbeatResponse{}, nil
	})

	_, err := m.Heartbeat(context.Background(), "")
	require.Error(t, err)
	_, err = m.Heartbeat(context.Background(), "")
	require.NoError(t, err)
}

func TestManager_SuppliedClientWrappersAreBoundedByAttemptTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	var calls atomic.Int32
	inner := sharddistributorexecutor.NewMockClient(ctrl)
	inner.EXPECT().Heartbeat(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ *types.ExecutorHeartbeatRequest, _ ...yarpc.CallOption) (*types.ExecutorHeartbeatResponse, error) {
			if calls.Add(1) == 1 {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return &types.ExecutorHeartbeatResponse{}, nil
		}).AnyTimes()

	var client sharddistributorexecutor.Client = timeout.NewShardDistributorExecutorClient(inner, timeout.ShardDistributorExecutorDefaultTimeout)
	client = retryable.NewShardDistributorExecutorClient(client, common.CreateShardDistributorServiceRetryPolicy(), common.IsServiceTransientError)

	state := NewMockStateProvider(ctrl)
	state.EXPECT().GetShardStatusReports().Return(nil).AnyTimes()
	state.EXPECT().GetMetadata().Return(nil).AnyTimes()
	m := NewManager(client, "ns", "exec", "", state, tally.NoopScope, tally.NoopScope, time.Second)

	start := time.Now()
	_, err := m.Heartbeat(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load())
	assert.Less(t, time.Since(start), 2*time.Second)
}
