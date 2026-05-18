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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

const (
	_testNamespaceFixed     = "test-fixed"
	_testNamespaceEphemeral = "test-ephemeral"
)

// newTestHandler creates a handlerImpl wired with a real shardBatcher backed by
// the provided store mock. The batcher is started and the handler is returned
// ready to use; callers should call Stop() when done.
func newTestHandler(t *testing.T, cfg config.ShardDistribution, mockStore *store.MockStore) *handlerImpl {
	t.Helper()
	handler := &handlerImpl{
		logger:               testlogger.New(t),
		shardDistributionCfg: cfg,
		cfg:                  newTestShardDistributorConfig(config.LoadBalancingModeNAIVE),
		storage:              mockStore,
		timeSource:           clock.NewRealTimeSource(),
	}
	handler.batcher = newShardBatcher(clock.NewRealTimeSource(), 10*time.Millisecond, handler.assignEphemeralBatch)
	handler.batcher.Start()
	t.Cleanup(handler.batcher.Stop)
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
			// ShardNotFound for an ephemeral namespace routes to the batcher, which
			// calls assignEphemeralBatch. This case validates the routing only;
			// detailed assignment behaviour is covered in TestAssignEphemeralBatch.
			name: "ShardNotFound_Ephemeral_RoutesToBatcher",
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
		{
			// A version conflict from AssignShards causes the batcher to return
			// ErrVersionConflict. getOrAssignEphemeralShard re-reads storage on
			// retry; here the concurrent winner has already written the assignment
			// so the second GetShardOwner call succeeds and no second batcher
			// submission is required.
			name: "Ephemeral_VersionConflict_ResolvedByStorageRead",
			request: &types.GetShardOwnerRequest{
				Namespace: _testNamespaceEphemeral,
				ShardKey:  "NON-EXISTING-SHARD",
			},
			setupMocks: func(mockStore *store.MockStore) {
				// Initial lookup — shard absent.
				mockStore.EXPECT().GetShardOwner(gomock.Any(), _testNamespaceEphemeral, "NON-EXISTING-SHARD").
					Return(nil, store.ErrShardNotFound)

				// Batcher fires: GetState + AssignShards returns a version conflict.
				mockStore.EXPECT().GetState(gomock.Any(), _testNamespaceEphemeral).Return(&store.NamespaceState{
					Executors:        map[string]store.HeartbeatState{"owner1": {Status: types.ExecutorStatusACTIVE}},
					ShardAssignments: map[string]store.AssignedState{"owner1": {AssignedShards: map[string]*types.ShardAssignment{}}},
				}, nil)
				mockStore.EXPECT().AssignShards(gomock.Any(), _testNamespaceEphemeral, gomock.Any(), gomock.Any()).
					Return(store.ErrVersionConflict)

				// Retry: re-read finds the shard already assigned by the concurrent winner.
				mockStore.EXPECT().GetShardOwner(gomock.Any(), _testNamespaceEphemeral, "NON-EXISTING-SHARD").
					Return(&store.ShardOwner{
						ExecutorID: "owner1",
						Metadata:   map[string]string{"ip": "127.0.0.1", "port": "1234"},
					}, nil)
			},
			expectedOwner: "owner1",
			expectedError: false,
		},
		{
			// A version conflict from AssignShards is retried; on the retry
			// GetShardOwner still returns ErrShardNotFound, so the batcher is
			// re-submitted and this time AssignShards succeeds.
			name: "Ephemeral_VersionConflict_RetriedAndSucceeds",
			request: &types.GetShardOwnerRequest{
				Namespace: _testNamespaceEphemeral,
				ShardKey:  "NON-EXISTING-SHARD",
			},
			setupMocks: func(mockStore *store.MockStore) {
				// Initial lookup — shard absent.
				mockStore.EXPECT().GetShardOwner(gomock.Any(), _testNamespaceEphemeral, "NON-EXISTING-SHARD").
					Return(nil, store.ErrShardNotFound)

				// First batcher attempt: version conflict.
				mockStore.EXPECT().GetState(gomock.Any(), _testNamespaceEphemeral).Return(&store.NamespaceState{
					Executors:        map[string]store.HeartbeatState{"owner1": {Status: types.ExecutorStatusACTIVE}},
					ShardAssignments: map[string]store.AssignedState{"owner1": {AssignedShards: map[string]*types.ShardAssignment{}}},
				}, nil)
				mockStore.EXPECT().AssignShards(gomock.Any(), _testNamespaceEphemeral, gomock.Any(), gomock.Any()).
					Return(fmt.Errorf("assign ephemeral shards: %w", store.ErrVersionConflict))

				// Retry re-read — shard still absent, so batcher is submitted again.
				mockStore.EXPECT().GetShardOwner(gomock.Any(), _testNamespaceEphemeral, "NON-EXISTING-SHARD").
					Return(nil, store.ErrShardNotFound)

				// Second batcher attempt: succeeds.
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

	updatesChan := make(chan map[*store.ShardOwner][]string, 1)
	unsubscribe := func() { close(updatesChan) }

	mockServer.EXPECT().Context().Return(ctx).AnyTimes()
	mockStorage.EXPECT().SubscribeToAssignmentChanges(gomock.Any(), "test-ns").Return(updatesChan, unsubscribe, nil)

	// Expect update send
	mockServer.EXPECT().Send(gomock.Any()).DoAndReturn(func(resp *types.WatchNamespaceStateResponse) error {
		require.Len(t, resp.Executors, 1)
		require.Equal(t, "executor-1", resp.Executors[0].ExecutorID)
		return nil
	})

	// Send update, then cancel
	go func() {
		time.Sleep(10 * time.Millisecond)
		updatesChan <- map[*store.ShardOwner][]string{
			{ExecutorID: "executor-1", Metadata: map[string]string{}}: {"shard-1"},
		}
		cancel()
	}()

	err := handler.WatchNamespaceState(&types.WatchNamespaceStateRequest{Namespace: "test-ns"}, mockServer)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
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
					{Name: "ns-fixed", Type: config.NamespaceTypeFixed, Mode: "onboarded", ShardNum: 32},
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
					{Name: "first", Type: config.NamespaceTypeFixed, Mode: "onboarded", ShardNum: 8},
					{Name: "second", Type: config.NamespaceTypeEphemeral, Mode: "distributed_pass"},
					{Name: "third", Type: config.NamespaceTypeFixed, Mode: "local_pass", ShardNum: 16},
				},
			},
			want: []*types.NamespaceConfig{
				{Name: "first", Type: "fixed", Mode: "onboarded", ShardNum: 8},
				{Name: "second", Type: "ephemeral", Mode: "distributed_pass"},
				{Name: "third", Type: "fixed", Mode: "local_pass", ShardNum: 16},
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

func TestDrainShardsStub(t *testing.T) {
	cfg := config.ShardDistribution{
		Namespaces: []config.Namespace{{Name: _testNamespaceFixed, Type: config.NamespaceTypeFixed, ShardNum: 32}},
	}
	ctrl := gomock.NewController(t)
	mockStorage := store.NewMockStore(ctrl)

	h := newTestHandler(t, cfg, mockStorage)
	resp, err := h.DrainShards(context.Background(), &types.DrainShardsRequest{
		Namespace: _testNamespaceFixed,
		ShardKeys: []string{"1"},
	})
	require.Nil(t, resp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not yet implemented")
}

func TestUndrainShardsStub(t *testing.T) {
	cfg := config.ShardDistribution{
		Namespaces: []config.Namespace{{Name: _testNamespaceFixed, Type: config.NamespaceTypeFixed, ShardNum: 32}},
	}
	ctrl := gomock.NewController(t)
	mockStorage := store.NewMockStore(ctrl)

	h := newTestHandler(t, cfg, mockStorage)
	resp, err := h.UndrainShards(context.Background(), &types.UndrainShardsRequest{
		Namespace: _testNamespaceFixed,
		ShardKeys: []string{"1"},
	})
	require.Nil(t, resp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not yet implemented")
}

func TestGetDrainedShardsStub(t *testing.T) {
	cfg := config.ShardDistribution{
		Namespaces: []config.Namespace{{Name: _testNamespaceFixed, Type: config.NamespaceTypeFixed, ShardNum: 32}},
	}
	ctrl := gomock.NewController(t)
	mockStorage := store.NewMockStore(ctrl)

	h := newTestHandler(t, cfg, mockStorage)
	resp, err := h.GetDrainedShards(context.Background(), &types.GetDrainedShardsRequest{
		Namespace: _testNamespaceFixed,
	})
	require.Nil(t, resp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not yet implemented")
}
