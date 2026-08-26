// The MIT License (MIT)

// Copyright (c) 2017-2020 Uber Technologies Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package handler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/ephemeralassigner"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

const (
	_testNamespaceFixed     = "test-fixed"
	_testNamespaceEphemeral = "test-ephemeral"
)

// newTestHandler creates a handlerImpl wired with a real ephemeral assigner backed
// by the provided store mock. The assigner is started and the handler is returned
// ready to use; callers should call Stop() when done.
func newTestHandler(t *testing.T, cfg config.ShardDistribution, mockStore *store.MockStore) *handlerImpl {
	t.Helper()
	assigner := ephemeralassigner.New(clock.NewRealTimeSource(), newTestShardDistributorConfig(config.LoadBalancingModeNAIVE), mockStore)
	handler := &handlerImpl{
		logger:               testlogger.New(t),
		shardDistributionCfg: cfg,
		storage:              mockStore,
		timeSource:           clock.NewRealTimeSource(),
		assigner:             assigner,
	}
	assigner.Start()
	t.Cleanup(assigner.Stop)
	return handler
}

func newTestShardDistributorConfig(mode string) *config.Config {
	return &config.Config{
		LoadBalancingMode: func(namespace string) string {
			return mode
		},
	}
}

func TestGetShardOwner(t *testing.T) {
	cfg := config.ShardDistribution{
		Namespaces: []config.Namespace{
			{
				Name:     _testNamespaceFixed,
				Type:     config.NamespaceTypeFixed,
				ShardNum: 32,
			},
			{
				Name: _testNamespaceEphemeral,
				Type: config.NamespaceTypeEphemeral,
			},
		},
	}

	tests := []struct {
		name           string
		request        *types.GetShardOwnerRequest
		setupMocks     func(mockStore *store.MockStore)
		expectedOwner  string
		expectedError  bool
		expectedErrMsg string
	}{
		{
			name: "InvalidNamespace",
			request: &types.GetShardOwnerRequest{
				Namespace: "namespace not found invalidNamespace",
				ShardKey:  "1",
			},
			expectedError:  true,
			expectedErrMsg: "namespace not found",
		},
		{
			name: "LookupError",
			request: &types.GetShardOwnerRequest{
				Namespace: _testNamespaceFixed,
				ShardKey:  "1",
			},
			setupMocks: func(mockStore *store.MockStore) {
				mockStore.EXPECT().GetShardOwner(gomock.Any(), _testNamespaceFixed, "1").Return(nil, errors.New("lookup error"))
			},
			expectedError:  true,
			expectedErrMsg: "lookup error",
		},
		{
			name: "Existing_Success_Fixed",
			request: &types.GetShardOwnerRequest{
				Namespace: _testNamespaceFixed,
				ShardKey:  "123",
			},
			setupMocks: func(mockStore *store.MockStore) {
				mockStore.EXPECT().GetShardOwner(gomock.Any(), _testNamespaceFixed, "123").Return(&store.ShardOwner{
					ExecutorID: "owner1",
					Metadata:   map[string]string{"ip": "127.0.0.1", "port": "1234"},
				}, nil)
			},
			expectedOwner: "owner1",
			expectedError: false,
		},
		{
			name: "ShardNotFound_Fixed",
			request: &types.GetShardOwnerRequest{
				Namespace: _testNamespaceFixed,
				ShardKey:  "NON-EXISTING-SHARD",
			},
			setupMocks: func(mockStore *store.MockStore) {
				mockStore.EXPECT().GetShardOwner(gomock.Any(), _testNamespaceFixed, "NON-EXISTING-SHARD").Return(nil, store.ErrShardNotFound)
			},
			expectedError:  true,
			expectedErrMsg: "shard not found",
		},
		{
			name: "Existing_Success_Ephemeral",
			request: &types.GetShardOwnerRequest{
				Namespace: _testNamespaceEphemeral,
				ShardKey:  "123",
			},
			setupMocks: func(mockStore *store.MockStore) {
				mockStore.EXPECT().GetShardOwner(gomock.Any(), _testNamespaceEphemeral, "123").Return(&store.ShardOwner{
					ExecutorID: "owner1",
					Metadata:   map[string]string{"ip": "127.0.0.1", "port": "1234"},
				}, nil)
			},
			expectedOwner: "owner1",
			expectedError: false,
		},
		{
			name: "Drained",
			request: &types.GetShardOwnerRequest{
				Namespace: _testNamespaceFixed,
				ShardKey:  "1",
			},
			setupMocks: func(mockStore *store.MockStore) {
				mockStore.EXPECT().GetShardOwner(gomock.Any(), _testNamespaceFixed, "1").Return(nil, store.ErrShardDrained)
			},
			expectedError:  true,
			expectedErrMsg: "shard drained",
		},
		{
			name: "ShardNotFound_Ephemeral_RoutesToAssigner",
			request: &types.GetShardOwnerRequest{
				Namespace: _testNamespaceEphemeral,
				ShardKey:  "NON-EXISTING-SHARD",
			},
			setupMocks: func(mockStore *store.MockStore) {
				mockStore.EXPECT().GetShardOwner(gomock.Any(), _testNamespaceEphemeral, "NON-EXISTING-SHARD").Return(nil, store.ErrShardNotFound)
				mockStore.EXPECT().GetState(gomock.Any(), _testNamespaceEphemeral).Return(&store.NamespaceState{
					Executors:        map[string]store.HeartbeatState{"owner1": {Status: types.ExecutorStatusACTIVE}},
					ShardAssignments: map[string]store.AssignedState{"owner1": {AssignedShards: map[string]*types.ShardAssignment{}}},
				}, nil)
				mockStore.EXPECT().AssignShards(gomock.Any(), _testNamespaceEphemeral, gomock.Any(), gomock.Any()).Return(nil)
				mockStore.EXPECT().GetExecutor(gomock.Any(), _testNamespaceEphemeral, "owner1").Return(&store.ShardOwner{
					ExecutorID: "owner1",
					Metadata:   map[string]string{"ip": "127.0.0.1", "port": "1234"},
				}, nil)
			},
			expectedOwner: "owner1",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStorage := store.NewMockStore(ctrl)

			handler := newTestHandler(t, cfg, mockStorage)

			if tt.setupMocks != nil {
				tt.setupMocks(mockStorage)
			}
			resp, err := handler.GetShardOwner(context.Background(), tt.request)
			if tt.expectedError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErrMsg)
				require.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedOwner, resp.Owner)
				require.Equal(t, tt.request.Namespace, resp.Namespace)
				expectedMetadata := map[string]string{"ip": "127.0.0.1", "port": "1234"}
				require.Equal(t, expectedMetadata, resp.Metadata)
			}
		})
	}
}

func TestInspectShard(t *testing.T) {
	cfg := config.ShardDistribution{
		Namespaces: []config.Namespace{
			{
				Name:     _testNamespaceFixed,
				Type:     config.NamespaceTypeFixed,
				ShardNum: 32,
			},
			{
				Name: _testNamespaceEphemeral,
				Type: config.NamespaceTypeEphemeral,
			},
		},
	}

	tests := []struct {
		name           string
		request        *types.GetShardOwnerRequest
		setupMocks     func(mockStore *store.MockStore)
		expectedOwner  string
		expectedError  bool
		expectedErrMsg string
	}{
		{
			name: "InvalidNamespace",
			request: &types.GetShardOwnerRequest{
				Namespace: "namespace not found invalidNamespace",
				ShardKey:  "1",
			},
			expectedError:  true,
			expectedErrMsg: "namespace not found",
		},
		{
			name: "LookupError",
			request: &types.GetShardOwnerRequest{
				Namespace: _testNamespaceFixed,
				ShardKey:  "1",
			},
			setupMocks: func(mockStore *store.MockStore) {
				mockStore.EXPECT().GetShardOwner(gomock.Any(), _testNamespaceFixed, "1").Return(nil, errors.New("lookup error"))
			},
			expectedError:  true,
			expectedErrMsg: "lookup error",
		},
		{
			name: "Existing_Success_Fixed",
			request: &types.GetShardOwnerRequest{
				Namespace: _testNamespaceFixed,
				ShardKey:  "123",
			},
			setupMocks: func(mockStore *store.MockStore) {
				mockStore.EXPECT().GetShardOwner(gomock.Any(), _testNamespaceFixed, "123").Return(&store.ShardOwner{
					ExecutorID: "owner1",
					Metadata:   map[string]string{"ip": "127.0.0.1", "port": "1234"},
				}, nil)
			},
			expectedOwner: "owner1",
			expectedError: false,
		},
		{
			name: "ShardNotFound_Fixed",
			request: &types.GetShardOwnerRequest{
				Namespace: _testNamespaceFixed,
				ShardKey:  "NON-EXISTING-SHARD",
			},
			setupMocks: func(mockStore *store.MockStore) {
				mockStore.EXPECT().GetShardOwner(gomock.Any(), _testNamespaceFixed, "NON-EXISTING-SHARD").Return(nil, store.ErrShardNotFound)
			},
			expectedError:  true,
			expectedErrMsg: "shard not found",
		},
		{
			name: "ShardNotFound_Ephemeral_DoesNotAssign",
			request: &types.GetShardOwnerRequest{
				Namespace: _testNamespaceEphemeral,
				ShardKey:  "NON-EXISTING-SHARD",
			},
			setupMocks: func(mockStore *store.MockStore) {
				mockStore.EXPECT().GetShardOwner(gomock.Any(), _testNamespaceEphemeral, "NON-EXISTING-SHARD").Return(nil, store.ErrShardNotFound)
			},
			expectedError:  true,
			expectedErrMsg: "shard not found",
		},
		{
			name: "Drained",
			request: &types.GetShardOwnerRequest{
				Namespace: _testNamespaceFixed,
				ShardKey:  "1",
			},
			setupMocks: func(mockStore *store.MockStore) {
				mockStore.EXPECT().GetShardOwner(gomock.Any(), _testNamespaceFixed, "1").Return(nil, store.ErrShardDrained)
			},
			expectedError:  true,
			expectedErrMsg: "shard drained",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStorage := store.NewMockStore(ctrl)

			handler := newTestHandler(t, cfg, mockStorage)

			if tt.setupMocks != nil {
				tt.setupMocks(mockStorage)
			}
			resp, err := handler.InspectShard(context.Background(), tt.request)
			if tt.expectedError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErrMsg)
				require.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedOwner, resp.Owner)
				require.Equal(t, tt.request.Namespace, resp.Namespace)
				expectedMetadata := map[string]string{"ip": "127.0.0.1", "port": "1234"}
				require.Equal(t, expectedMetadata, resp.Metadata)
			}
		})
	}
}

func TestWatchNamespaceState(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := testlogger.New(t)
	mockStorage := store.NewMockStore(ctrl)
	mockServer := NewMockWatchNamespaceStateServer(ctrl)

	cfg := config.ShardDistribution{
		Namespaces: []config.Namespace{
			{Name: "test-ns", Type: config.NamespaceTypeFixed, ShardNum: 2},
		},
	}

	handler := &handlerImpl{
		logger:               logger,
		shardDistributionCfg: cfg,
		storage:              mockStorage,
		startWG:              sync.WaitGroup{},
	}

	ctx, cancel := context.WithCancel(context.Background())

	notifyChan := make(chan struct{}, 1)
	unsubscribe := func() { close(notifyChan) }

	mockServer.EXPECT().Context().Return(ctx).AnyTimes()
	mockStorage.EXPECT().SubscribeToAssignmentChanges(ctx, "test-ns").Return(notifyChan, unsubscribe, nil)

	// this state should be retrieved and sent at the start of WatchNamespaceState
	getState1 := mockStorage.EXPECT().GetShardAssignments("test-ns").Return(
		store.AssignmentSnapshot{
			ExecutorToShards: map[*store.ShardOwner][]string{
				{ExecutorID: "executor-1", Metadata: map[string]string{}}: {"shard-1"},
			},
			DrainedShards: map[string]struct{}{"shard-drained": {}},
		},
		nil,
	)

	send1 := mockServer.EXPECT().Send(gomock.Any()).DoAndReturn(func(resp *types.WatchNamespaceStateResponse) error {
		require.Len(t, resp.Executors, 1)
		assert.Equal(t, "executor-1", resp.Executors[0].ExecutorID)
		assert.Equal(t, []string{"shard-drained"}, resp.DrainedShardKeys)
		return nil
	})

	// now simulate the state has been changed (after notification)
	getState2 := mockStorage.EXPECT().GetShardAssignments("test-ns").Return(
		store.AssignmentSnapshot{
			ExecutorToShards: map[*store.ShardOwner][]string{
				{ExecutorID: "executor-1", Metadata: map[string]string{}}: {"shard-1"},
				{ExecutorID: "executor-2", Metadata: map[string]string{}}: {"shard-2"},
			},
			DrainedShards: map[string]struct{}{"shard-10": {}, "shard-2": {}},
		},
		nil,
	)

	send2 := mockServer.EXPECT().Send(gomock.Any()).DoAndReturn(func(resp *types.WatchNamespaceStateResponse) error {
		cancel()

		require.Len(t, resp.Executors, 2)
		executors := []string{resp.Executors[0].ExecutorID, resp.Executors[1].ExecutorID}
		assert.Contains(t, executors, "executor-1")
		assert.Contains(t, executors, "executor-2")
		assert.Equal(t, []string{"shard-10", "shard-2"}, resp.DrainedShardKeys)
		return nil
	})

	gomock.InOrder(
		getState1,
		send1,
		getState2,
		send2,
	)

	notifyChan <- struct{}{}

	err := handler.WatchNamespaceState(&types.WatchNamespaceStateRequest{Namespace: "test-ns"}, mockServer)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestWatchNamespaceStateStopsOnHandlerStop(t *testing.T) {
	const watchStopTimeout = 5 * time.Second

	ctrl := gomock.NewController(t)
	logger := testlogger.New(t)
	mockStorage := store.NewMockStore(ctrl)
	mockServer := NewMockWatchNamespaceStateServer(ctrl)

	cfg := config.ShardDistribution{
		Namespaces: []config.Namespace{
			{Name: "test-ns", Type: config.NamespaceTypeFixed, ShardNum: 2},
		},
	}

	rawHandler := NewHandler(logger, clock.NewRealTimeSource(), cfg, newTestShardDistributorConfig(config.LoadBalancingModeNAIVE), mockStorage)
	handler := rawHandler.(*handlerImpl)
	handler.Start()

	notifyChan := make(chan struct{}, 1)
	unsubscribe := func() { close(notifyChan) }
	serverCtx := context.Background()

	mockServer.EXPECT().Context().Return(serverCtx).AnyTimes()
	mockStorage.EXPECT().SubscribeToAssignmentChanges(serverCtx, "test-ns").Return(notifyChan, unsubscribe, nil)

	mockStorage.EXPECT().GetShardAssignments("test-ns").Return(
		store.AssignmentSnapshot{
			ExecutorToShards: map[*store.ShardOwner][]string{},
			DrainedShards:    map[string]struct{}{},
		},
		nil,
	)
	mockServer.EXPECT().Send(gomock.Any()).Return(nil)

	errCh := make(chan error, 1)
	go func() {
		errCh <- handler.WatchNamespaceState(&types.WatchNamespaceStateRequest{Namespace: "test-ns"}, mockServer)
	}()

	handler.Stop()

	select {
	case err := <-errCh:
		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(watchStopTimeout):
		t.Fatal("WatchNamespaceState did not stop after handler.Stop")
	}
}

func TestGetNamespaceState(t *testing.T) {
	cfg := config.ShardDistribution{
		Namespaces: []config.Namespace{
			{Name: _testNamespaceFixed, Type: config.NamespaceTypeFixed, ShardNum: 32},
		},
	}

	tests := []struct {
		name            string
		request         *types.GetNamespaceStateRequest
		setupMocks      func(*store.MockStore)
		wantErrContains string
	}{
		{
			name:            "unknown_namespace",
			request:         &types.GetNamespaceStateRequest{Namespace: "missing"},
			wantErrContains: "namespace not found",
		},
		{
			name:    "get_state_error",
			request: &types.GetNamespaceStateRequest{Namespace: _testNamespaceFixed},
			setupMocks: func(m *store.MockStore) {
				m.EXPECT().GetState(gomock.Any(), _testNamespaceFixed).Return(nil, errors.New("etcd is down"))
			},
			wantErrContains: "failed to get namespace state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStorage := store.NewMockStore(ctrl)
			if tt.setupMocks != nil {
				tt.setupMocks(mockStorage)
			}
			h := newTestHandler(t, cfg, mockStorage)
			resp, err := h.GetNamespaceState(context.Background(), tt.request)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErrContains)
			require.Nil(t, resp)
		})
	}
}

func TestGetNamespaceState_successMultipleExecutors(t *testing.T) {
	cfg := config.ShardDistribution{
		Namespaces: []config.Namespace{
			{Name: _testNamespaceFixed, Type: config.NamespaceTypeFixed, ShardNum: 32},
		},
	}
	now := time.Unix(1000, 0).UTC()

	ctrl := gomock.NewController(t)
	mockStorage := store.NewMockStore(ctrl)
	mockStorage.EXPECT().GetState(gomock.Any(), _testNamespaceFixed).Return(&store.NamespaceState{
		Executors: map[string]store.HeartbeatState{
			"executor1": {Status: types.ExecutorStatusACTIVE, LastHeartbeat: now, Metadata: map[string]string{"ip": "127.0.0.1", "port": "1234"}},
			"executor2": {},
		},
		ShardAssignments: map[string]store.AssignedState{
			"executor1": {
				AssignedShards: map[string]*types.ShardAssignment{
					"shard1": {Status: types.AssignmentStatusREADY},
					"shard2": {Status: types.AssignmentStatusREADY},
				},
				ModRevision: 42,
			},
			"executor2": {
				AssignedShards: map[string]*types.ShardAssignment{
					"shard3": nil,
				},
				ModRevision: 7,
			},
		},
	}, nil)

	h := newTestHandler(t, cfg, mockStorage)
	resp, err := h.GetNamespaceState(context.Background(), &types.GetNamespaceStateRequest{Namespace: _testNamespaceFixed})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, _testNamespaceFixed, resp.Namespace)
	require.Len(t, resp.Executors, 2)

	byID := make(map[string]*types.NamespaceExecutorState, len(resp.Executors))
	for _, executor := range resp.Executors {
		byID[executor.ExecutorID] = executor
	}
	e1 := byID["executor1"]
	require.NotNil(t, e1)
	require.Equal(t, types.ExecutorStatusACTIVE, e1.Status)
	require.Len(t, e1.AssignedShards, 2)
	shardKeys := make([]string, 0, len(e1.AssignedShards))
	for _, sh := range e1.AssignedShards {
		shardKeys = append(shardKeys, sh.ShardKey)
		require.Equal(t, int64(42), sh.AssignedStateModRevision)
		require.Equal(t, types.AssignmentStatusREADY, sh.AssignmentStatus)
	}
	require.ElementsMatch(t, []string{"shard2", "shard1"}, shardKeys)

	e2 := byID["executor2"]
	require.NotNil(t, e2)
	require.Len(t, e2.AssignedShards, 1)
	require.Equal(t, "shard3", e2.AssignedShards[0].ShardKey)
	require.Equal(t, types.AssignmentStatusINVALID, e2.AssignedShards[0].AssignmentStatus)
}

func TestForceResetNamespace(t *testing.T) {
	cfg := config.ShardDistribution{
		Namespaces: []config.Namespace{
			{Name: _testNamespaceFixed, Type: config.NamespaceTypeFixed, ShardNum: 32},
		},
	}

	tests := []struct {
		name            string
		request         *types.ForceResetNamespaceRequest
		setupMocks      func(*store.MockStore)
		wantDeletedKeys int64
		wantErrContains string
	}{
		{
			name:            "unknown_namespace",
			request:         &types.ForceResetNamespaceRequest{Namespace: "missing"},
			wantErrContains: "namespace not found",
		},
		{
			name:    "store_error",
			request: &types.ForceResetNamespaceRequest{Namespace: _testNamespaceFixed},
			setupMocks: func(m *store.MockStore) {
				m.EXPECT().ResetNamespace(gomock.Any(), _testNamespaceFixed).
					Return(int64(0), errors.New("etcd is down"))
			},
			wantErrContains: "failed to reset namespace",
		},
		{
			name:    "success",
			request: &types.ForceResetNamespaceRequest{Namespace: _testNamespaceFixed},
			setupMocks: func(m *store.MockStore) {
				m.EXPECT().ResetNamespace(gomock.Any(), _testNamespaceFixed).
					Return(int64(17), nil)
			},
			wantDeletedKeys: 17,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStorage := store.NewMockStore(ctrl)
			if tt.setupMocks != nil {
				tt.setupMocks(mockStorage)
			}
			h := newTestHandler(t, cfg, mockStorage)
			resp, err := h.ForceResetNamespace(context.Background(), tt.request)
			if tt.wantErrContains != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrContains)
				require.Nil(t, resp)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Equal(t, tt.wantDeletedKeys, resp.DeletedKeys)
		})
	}
}

func TestGetNamespaceState_heartbeatWithoutAssignments(t *testing.T) {
	cfg := config.ShardDistribution{
		Namespaces: []config.Namespace{
			{Name: _testNamespaceFixed, Type: config.NamespaceTypeFixed, ShardNum: 32},
		},
	}
	now := time.Unix(1000, 0).UTC()

	ctrl := gomock.NewController(t)
	mockStorage := store.NewMockStore(ctrl)
	mockStorage.EXPECT().GetState(gomock.Any(), _testNamespaceFixed).Return(&store.NamespaceState{
		Executors: map[string]store.HeartbeatState{
			"exec-heartbeat-only": {
				Status:        types.ExecutorStatusDRAINING,
				LastHeartbeat: now,
				Metadata:      map[string]string{"ip": "127.0.0.1", "port": "1234"},
			},
		},
		ShardAssignments: map[string]store.AssignedState{},
	}, nil)

	h := newTestHandler(t, cfg, mockStorage)
	resp, err := h.GetNamespaceState(context.Background(), &types.GetNamespaceStateRequest{Namespace: _testNamespaceFixed})
	require.NoError(t, err)
	require.Len(t, resp.Executors, 1)
	ex := resp.Executors[0]
	require.Equal(t, "exec-heartbeat-only", ex.ExecutorID)
	require.Equal(t, types.ExecutorStatusDRAINING, ex.Status)
	require.Equal(t, now, ex.LastHeartbeat)
	require.Empty(t, ex.AssignedShards)
}

func TestGetExecutorState(t *testing.T) {
	cfg := config.ShardDistribution{
		Namespaces: []config.Namespace{
			{Name: _testNamespaceFixed, Type: config.NamespaceTypeFixed, ShardNum: 32},
		},
	}

	tests := []struct {
		name            string
		request         *types.GetExecutorStateRequest
		setupMocks      func(*store.MockStore)
		wantErrContains string
	}{
		{
			name:            "unknown_namespace",
			request:         &types.GetExecutorStateRequest{Namespace: "missing", ExecutorID: "executor1"},
			wantErrContains: "namespace not found",
		},
		{
			name:    "executor_not_found",
			request: &types.GetExecutorStateRequest{Namespace: _testNamespaceFixed, ExecutorID: "executor1"},
			setupMocks: func(m *store.MockStore) {
				m.EXPECT().GetExecutorState(gomock.Any(), _testNamespaceFixed, "executor1").
					Return(store.ExecutorState{}, store.ErrExecutorNotFound)
			},
			wantErrContains: "executor not found",
		},
		{
			name:    "get_executor_state_error",
			request: &types.GetExecutorStateRequest{Namespace: _testNamespaceFixed, ExecutorID: "executor1"},
			setupMocks: func(m *store.MockStore) {
				m.EXPECT().GetExecutorState(gomock.Any(), _testNamespaceFixed, "executor1").
					Return(store.ExecutorState{}, errors.New("etcd is down"))
			},
			wantErrContains: "failed to get executor state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStorage := store.NewMockStore(ctrl)
			if tt.setupMocks != nil {
				tt.setupMocks(mockStorage)
			}
			h := newTestHandler(t, cfg, mockStorage)
			resp, err := h.GetExecutorState(context.Background(), tt.request)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErrContains)
			require.Nil(t, resp)
		})
	}
}

func TestGetExecutorState_success(t *testing.T) {
	cfg := config.ShardDistribution{
		Namespaces: []config.Namespace{
			{Name: _testNamespaceFixed, Type: config.NamespaceTypeFixed, ShardNum: 32},
		},
	}
	now := time.Unix(1000, 0).UTC()

	ctrl := gomock.NewController(t)
	mockStorage := store.NewMockStore(ctrl)
	mockStorage.EXPECT().GetExecutorState(gomock.Any(), _testNamespaceFixed, "executor1").Return(
		store.ExecutorState{
			Heartbeat: &store.HeartbeatState{
				Status:        types.ExecutorStatusACTIVE,
				LastHeartbeat: now,
				Metadata:      map[string]string{"ip": "127.0.0.1", "port": "1234"},
			},
			Assignment: &store.AssignedState{
				AssignedShards: map[string]*types.ShardAssignment{
					"shard1": {Status: types.AssignmentStatusREADY},
					"shard2": nil,
				},
				ModRevision: 42,
			},
		},
		nil,
	)

	h := newTestHandler(t, cfg, mockStorage)
	resp, err := h.GetExecutorState(context.Background(), &types.GetExecutorStateRequest{
		Namespace:  _testNamespaceFixed,
		ExecutorID: "executor1",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, _testNamespaceFixed, resp.Namespace)
	require.NotNil(t, resp.Executor)
	require.Equal(t, "executor1", resp.Executor.ExecutorID)
	require.Equal(t, types.ExecutorStatusACTIVE, resp.Executor.Status)
	require.Equal(t, now, resp.Executor.LastHeartbeat)
	require.Equal(t, map[string]string{"ip": "127.0.0.1", "port": "1234"}, resp.Executor.Metadata)
	require.Len(t, resp.Executor.AssignedShards, 2)

	byKey := make(map[string]*types.ExecutorAssignedShardState, len(resp.Executor.AssignedShards))
	for _, sh := range resp.Executor.AssignedShards {
		byKey[sh.ShardKey] = sh
		require.Equal(t, int64(42), sh.AssignedStateModRevision)
	}
	require.Equal(t, types.AssignmentStatusREADY, byKey["shard1"].AssignmentStatus)
	require.Equal(t, types.AssignmentStatusINVALID, byKey["shard2"].AssignmentStatus)
}

func TestGetExecutorState_noAssignedState(t *testing.T) {
	cfg := config.ShardDistribution{
		Namespaces: []config.Namespace{
			{Name: _testNamespaceFixed, Type: config.NamespaceTypeFixed, ShardNum: 32},
		},
	}
	now := time.Unix(1000, 0).UTC()

	ctrl := gomock.NewController(t)
	mockStorage := store.NewMockStore(ctrl)
	mockStorage.EXPECT().GetExecutorState(gomock.Any(), _testNamespaceFixed, "executor1").Return(
		store.ExecutorState{
			Heartbeat: &store.HeartbeatState{
				Status:        types.ExecutorStatusDRAINING,
				LastHeartbeat: now,
			},
		},
		nil,
	)

	h := newTestHandler(t, cfg, mockStorage)
	resp, err := h.GetExecutorState(context.Background(), &types.GetExecutorStateRequest{
		Namespace:  _testNamespaceFixed,
		ExecutorID: "executor1",
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Executor)
	require.Equal(t, types.ExecutorStatusDRAINING, resp.Executor.Status)
	require.Empty(t, resp.Executor.AssignedShards)
}

func TestListNamespaces(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.ShardDistribution
		want []*types.NamespaceConfig
	}{
		{
			name: "empty config returns empty slice (not nil)",
			cfg:  config.ShardDistribution{},
			want: []*types.NamespaceConfig{},
		},
		{
			name: "single fixed namespace",
			cfg: config.ShardDistribution{
				Namespaces: []config.Namespace{
					{Name: "ns-fixed", Type: config.NamespaceTypeFixed, ShardNum: 32},
				},
			},
			want: []*types.NamespaceConfig{
				{Name: "ns-fixed", Type: "fixed", Mode: "onboarded", ShardNum: 32},
			},
		},
		{
			name: "preserves order and surfaces both fixed and ephemeral",
			cfg: config.ShardDistribution{
				Namespaces: []config.Namespace{
					{Name: "first", Type: config.NamespaceTypeFixed, ShardNum: 8},
					{Name: "second", Type: config.NamespaceTypeEphemeral},
					{Name: "third", Type: config.NamespaceTypeFixed, ShardNum: 16},
				},
			},
			want: []*types.NamespaceConfig{
				{Name: "first", Type: "fixed", Mode: "onboarded", ShardNum: 8},
				{Name: "second", Type: "ephemeral", Mode: "onboarded"},
				{Name: "third", Type: "fixed", Mode: "onboarded", ShardNum: 16},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStorage := store.NewMockStore(ctrl) // ListNamespaces does not call storage.

			h := newTestHandler(t, tt.cfg, mockStorage)
			resp, err := h.ListNamespaces(context.Background(), &types.ListNamespacesRequest{})
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Equal(t, tt.want, resp.Namespaces)
		})
	}
}

func TestDrainShards(t *testing.T) {
	cfg := config.ShardDistribution{
		Namespaces: []config.Namespace{{Name: _testNamespaceFixed, Type: config.NamespaceTypeFixed, ShardNum: 32}},
	}

	tests := []struct {
		name            string
		request         *types.DrainShardsRequest
		setupMocks      func(*store.MockStore)
		wantErr         error
		wantErrContains string
	}{
		{
			name:    "unknown namespace",
			request: &types.DrainShardsRequest{Namespace: "missing", ShardKeys: []string{"1"}},
			wantErr: &types.NamespaceNotFoundError{Namespace: "missing"},
		},
		{
			name:    "no shard keys",
			request: &types.DrainShardsRequest{Namespace: _testNamespaceFixed},
			wantErr: &types.BadRequestError{Message: "shard keys must not be empty"},
		},
		{
			name:    "empty shard key",
			request: &types.DrainShardsRequest{Namespace: _testNamespaceFixed, ShardKeys: []string{"1", ""}},
			wantErr: &types.BadRequestError{Message: `invalid shard key "": must be non-empty and must not contain '/'`},
		},
		{
			name:    "shard key with separator",
			request: &types.DrainShardsRequest{Namespace: _testNamespaceFixed, ShardKeys: []string{"a/b"}},
			wantErr: &types.BadRequestError{Message: `invalid shard key "a/b": must be non-empty and must not contain '/'`},
		},
		{
			name:    "store error",
			request: &types.DrainShardsRequest{Namespace: _testNamespaceFixed, ShardKeys: []string{"1"}},
			setupMocks: func(m *store.MockStore) {
				m.EXPECT().DrainShards(gomock.Any(), _testNamespaceFixed, []string{"1"}).
					Return(errors.New("etcd is down"))
			},
			wantErrContains: "failed to drain shards",
		},
		{
			name:    "success returns whole drained set",
			request: &types.DrainShardsRequest{Namespace: _testNamespaceFixed, ShardKeys: []string{"1", "2"}},
			setupMocks: func(m *store.MockStore) {
				m.EXPECT().DrainShards(gomock.Any(), _testNamespaceFixed, []string{"1", "2"}).
					Return(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStorage := store.NewMockStore(ctrl)
			if tt.setupMocks != nil {
				tt.setupMocks(mockStorage)
			}

			h := newTestHandler(t, cfg, mockStorage)
			err := h.DrainShards(context.Background(), tt.request)

			if tt.wantErr != nil {
				require.Equal(t, tt.wantErr, err)
				return
			}
			if tt.wantErrContains != "" {
				require.ErrorContains(t, err, tt.wantErrContains)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestUndrainShards(t *testing.T) {
	cfg := config.ShardDistribution{
		Namespaces: []config.Namespace{{Name: _testNamespaceFixed, Type: config.NamespaceTypeFixed, ShardNum: 32}},
	}

	tests := []struct {
		name            string
		request         *types.UndrainShardsRequest
		setupMocks      func(*store.MockStore)
		wantUndrained   []string
		wantErr         error
		wantErrContains string
	}{
		{
			name:    "unknown namespace",
			request: &types.UndrainShardsRequest{Namespace: "missing", ShardKeys: []string{"1"}},
			wantErr: &types.NamespaceNotFoundError{Namespace: "missing"},
		},
		{
			name:    "no shard keys",
			request: &types.UndrainShardsRequest{Namespace: _testNamespaceFixed},
			wantErr: &types.BadRequestError{Message: "shard keys must not be empty"},
		},
		{
			name:    "store error",
			request: &types.UndrainShardsRequest{Namespace: _testNamespaceFixed, ShardKeys: []string{"1"}},
			setupMocks: func(m *store.MockStore) {
				m.EXPECT().UndrainShards(gomock.Any(), _testNamespaceFixed, []string{"1"}).
					Return(nil, errors.New("etcd is down"))
			},
			wantErrContains: "failed to undrain shards",
		},
		{
			name:    "success reports only what was removed",
			request: &types.UndrainShardsRequest{Namespace: _testNamespaceFixed, ShardKeys: []string{"1", "2"}},
			setupMocks: func(m *store.MockStore) {
				m.EXPECT().UndrainShards(gomock.Any(), _testNamespaceFixed, []string{"1", "2"}).
					Return([]string{"1"}, nil)
			},
			wantUndrained: []string{"1"},
		},
		{
			name:    "nothing was drained",
			request: &types.UndrainShardsRequest{Namespace: _testNamespaceFixed, ShardKeys: []string{"1"}},
			setupMocks: func(m *store.MockStore) {
				m.EXPECT().UndrainShards(gomock.Any(), _testNamespaceFixed, []string{"1"}).
					Return(nil, nil)
			},
			wantUndrained: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStorage := store.NewMockStore(ctrl)
			if tt.setupMocks != nil {
				tt.setupMocks(mockStorage)
			}

			h := newTestHandler(t, cfg, mockStorage)
			resp, err := h.UndrainShards(context.Background(), tt.request)

			if tt.wantErr != nil {
				require.Nil(t, resp)
				require.Equal(t, tt.wantErr, err)
				return
			}
			if tt.wantErrContains != "" {
				require.Nil(t, resp)
				require.ErrorContains(t, err, tt.wantErrContains)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Equal(t, tt.wantUndrained, resp.UndrainedShardKeys)
		})
	}
}

func TestGetDrainedShards(t *testing.T) {
	cfg := config.ShardDistribution{
		Namespaces: []config.Namespace{{Name: _testNamespaceFixed, Type: config.NamespaceTypeFixed, ShardNum: 32}},
	}

	tests := []struct {
		name            string
		request         *types.GetDrainedShardsRequest
		setupMocks      func(*store.MockStore)
		wantShardKeys   []string
		wantErr         error
		wantErrContains string
	}{
		{
			name:    "unknown namespace",
			request: &types.GetDrainedShardsRequest{Namespace: "missing"},
			wantErr: &types.NamespaceNotFoundError{Namespace: "missing"},
		},
		{
			name:    "store error",
			request: &types.GetDrainedShardsRequest{Namespace: _testNamespaceFixed},
			setupMocks: func(m *store.MockStore) {
				m.EXPECT().GetDrainedShards(gomock.Any(), _testNamespaceFixed).
					Return(nil, errors.New("etcd is down"))
			},
			wantErrContains: "failed to get drained shards",
		},
		{
			name:    "nothing drained",
			request: &types.GetDrainedShardsRequest{Namespace: _testNamespaceFixed},
			setupMocks: func(m *store.MockStore) {
				m.EXPECT().GetDrainedShards(gomock.Any(), _testNamespaceFixed).Return(nil, nil)
			},
			wantShardKeys: nil,
		},
		{
			name:    "success",
			request: &types.GetDrainedShardsRequest{Namespace: _testNamespaceFixed},
			setupMocks: func(m *store.MockStore) {
				m.EXPECT().GetDrainedShards(gomock.Any(), _testNamespaceFixed).
					Return([]string{"1", "7"}, nil)
			},
			wantShardKeys: []string{"1", "7"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStorage := store.NewMockStore(ctrl)
			if tt.setupMocks != nil {
				tt.setupMocks(mockStorage)
			}

			h := newTestHandler(t, cfg, mockStorage)
			resp, err := h.GetDrainedShards(context.Background(), tt.request)

			if tt.wantErr != nil {
				require.Nil(t, resp)
				require.Equal(t, tt.wantErr, err)
				return
			}
			if tt.wantErrContains != "" {
				require.Nil(t, resp)
				require.ErrorContains(t, err, tt.wantErrContains)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Equal(t, _testNamespaceFixed, resp.Namespace)
			require.Equal(t, tt.wantShardKeys, resp.ShardKeys)
		})
	}
}
