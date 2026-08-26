package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap/zapcore"

	"github.com/cadence-workflow/shard-manager/common"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	metricmocks "github.com/cadence-workflow/shard-manager/common/metrics/mocks"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

func TestHeartbeat(t *testing.T) {
	ctx := context.Background()
	namespace := "test-namespace"
	executorID := "test-executor"
	now := time.Now().UTC()

	// Test Case 1: First Heartbeat
	t.Run("FirstHeartbeat", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		mockTimeSource := clock.NewMockedTimeSourceAt(now)
		handler := newTestExecutorHandler(t, mockStore, mockTimeSource)

		req := &types.ExecutorHeartbeatRequest{
			Namespace:  namespace,
			ExecutorID: executorID,
			Status:     types.ExecutorStatusACTIVE,
		}

		mockStore.EXPECT().GetExecutorState(gomock.Any(), namespace, executorID).Return(store.ExecutorState{}, store.ErrExecutorNotFound)
		mockStore.EXPECT().RecordHeartbeat(gomock.Any(), namespace, executorID, store.HeartbeatState{
			LastHeartbeat: now,
			Status:        types.ExecutorStatusACTIVE,
		})
		_, err := handler.Heartbeat(ctx, req)
		require.NoError(t, err)
	})

	// Test Case 2: Subsequent heartbeat records a new heartbeat
	t.Run("SubsequentHeartbeatRecords", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		mockTimeSource := clock.NewMockedTimeSourceAt(now)
		handler := newTestExecutorHandler(t, mockStore, mockTimeSource)

		reports := map[string]*types.ShardStatusReport{
			"shard-1": {Status: types.ShardStatusREADY, ShardLoad: 12.3},
		}
		req := &types.ExecutorHeartbeatRequest{
			Namespace:          namespace,
			ExecutorID:         executorID,
			Status:             types.ExecutorStatusACTIVE,
			ShardStatusReports: reports,
		}

		previousHeartbeat := store.HeartbeatState{
			LastHeartbeat: now,
			Status:        types.ExecutorStatusACTIVE,
		}

		mockStore.EXPECT().GetExecutorState(gomock.Any(), namespace, executorID).Return(store.ExecutorState{
			Heartbeat: &previousHeartbeat,
		}, nil)
		mockStore.EXPECT().RecordHeartbeat(gomock.Any(), namespace, executorID, store.HeartbeatState{
			LastHeartbeat:  now,
			Status:         types.ExecutorStatusACTIVE,
			ReportedShards: reports,
		})

		_, err := handler.Heartbeat(ctx, req)
		require.NoError(t, err)
	})

	// Test Case 3: Status Change (with update)
	t.Run("StatusChange", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		mockTimeSource := clock.NewMockedTimeSourceAt(now)
		handler := newTestExecutorHandler(t, mockStore, mockTimeSource)

		req := &types.ExecutorHeartbeatRequest{
			Namespace:  namespace,
			ExecutorID: executorID,
			Status:     types.ExecutorStatusDRAINING, // Status changed
		}

		previousHeartbeat := store.HeartbeatState{
			LastHeartbeat: now,
			Status:        types.ExecutorStatusACTIVE,
		}

		mockStore.EXPECT().GetExecutorState(gomock.Any(), namespace, executorID).Return(store.ExecutorState{
			Heartbeat: &previousHeartbeat,
		}, nil)
		mockStore.EXPECT().RecordHeartbeat(gomock.Any(), namespace, executorID, store.HeartbeatState{
			LastHeartbeat: now,
			Status:        types.ExecutorStatusDRAINING,
		})
		_, err := handler.Heartbeat(ctx, req)
		require.NoError(t, err)
	})

	// Test Case 4: Storage Error
	t.Run("StorageError", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		mockTimeSource := clock.NewMockedTimeSource()
		handler := newTestExecutorHandler(t, mockStore, mockTimeSource)

		req := &types.ExecutorHeartbeatRequest{
			Namespace:  namespace,
			ExecutorID: executorID,
			Status:     types.ExecutorStatusACTIVE,
		}

		expectedErr := errors.New("storage is down")
		mockStore.EXPECT().GetExecutorState(gomock.Any(), namespace, executorID).Return(store.ExecutorState{}, expectedErr)

		_, err := handler.Heartbeat(ctx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), expectedErr.Error())
	})

	t.Run("StatisticsErrorDoesNotFailHeartbeat", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		mockTimeSource := clock.NewMockedTimeSourceAt(now)
		logger, logs := testlogger.NewObserved(t)
		handler := NewExecutorHandler(logger, mockStore, mockTimeSource, newTestShardDistributorConfig(config.LoadBalancingModeGREEDY), metrics.NoopClient)

		const assignmentModRevision = int64(42)
		reports := map[string]*types.ShardStatusReport{
			"shard-1": {ShardLoad: 12.3},
		}

		req := &types.ExecutorHeartbeatRequest{
			Namespace:          namespace,
			ExecutorID:         executorID,
			Status:             types.ExecutorStatusACTIVE,
			ShardStatusReports: reports,
		}
		assignedState := &store.AssignedState{
			AssignedShards: map[string]*types.ShardAssignment{
				"shard-1": {Status: types.AssignmentStatusREADY},
			},
			ModRevision: assignmentModRevision,
		}

		mockStore.EXPECT().GetExecutorState(gomock.Any(), namespace, executorID).Return(store.ExecutorState{
			Assignment: assignedState,
		}, nil)
		mockStore.EXPECT().RecordHeartbeat(gomock.Any(), namespace, executorID, gomock.Any()).Return(nil)
		mockStore.EXPECT().RecordShardStatistics(gomock.Any(), namespace, executorID, assignmentModRevision, gomock.Any()).Return(assert.AnError)

		response, err := handler.Heartbeat(ctx, req)
		require.NoError(t, err)
		require.Equal(t, assignedState.AssignedShards, response.ShardAssignments)

		entries := logs.FilterMessage("failed to record shard statistics").All()
		require.Len(t, entries, 1)
		assert.Equal(t, zapcore.WarnLevel, entries[0].Level)
	})

	// Test Case 9: Heartbeat with metadata validation failure - too many keys
	t.Run("MetadataValidationTooManyKeys", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		mockTimeSource := clock.NewMockedTimeSourceAt(now)
		handler := newTestExecutorHandler(t, mockStore, mockTimeSource)

		// Create metadata with more than max allowed keys
		metadata := make(map[string]string)
		for i := 0; i < _maxMetadataKeys+1; i++ {
			metadata[string(rune('a'+i))] = "value"
		}

		req := &types.ExecutorHeartbeatRequest{
			Namespace:  namespace,
			ExecutorID: executorID,
			Status:     types.ExecutorStatusACTIVE,
			Metadata:   metadata,
		}

		mockStore.EXPECT().GetExecutorState(gomock.Any(), namespace, executorID).Return(store.ExecutorState{}, store.ErrExecutorNotFound)

		_, err := handler.Heartbeat(ctx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid metadata: metadata has 33 keys, which exceeds the maximum of 32")
	})

}

func TestValidateMetadata(t *testing.T) {
	// Helper function to generate metadata with N keys
	makeMetadataWithKeys := func(n int) map[string]string {
		metadata := make(map[string]string)
		for i := 0; i < n; i++ {
			metadata[string(rune('a'+i))] = "value"
		}
		return metadata
	}

	testCases := []struct {
		name           string
		metadata       map[string]string
		expectError    bool
		errorSubstring string
	}{
		{
			name: "ValidMetadata",
			metadata: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expectError: false,
		},
		{
			name:        "EmptyMetadata",
			metadata:    map[string]string{},
			expectError: false,
		},
		{
			name:        "NilMetadata",
			metadata:    nil,
			expectError: false,
		},
		{
			name:           "TooManyKeys",
			metadata:       makeMetadataWithKeys(_maxMetadataKeys + 1),
			expectError:    true,
			errorSubstring: "exceeds the maximum of 32",
		},
		{
			name:        "ExactlyMaxKeys",
			metadata:    makeMetadataWithKeys(_maxMetadataKeys),
			expectError: false,
		},
		{
			name: "KeyTooLong",
			metadata: map[string]string{
				string(make([]byte, _maxMetadataKeyLength+1)): "value",
			},
			expectError:    true,
			errorSubstring: "exceeds the maximum of 128",
		},
		{
			name: "KeyExactlyMaxLength",
			metadata: map[string]string{
				string(make([]byte, _maxMetadataKeyLength)): "value",
			},
			expectError: false,
		},
		{
			name: "ValueTooLarge",
			metadata: map[string]string{
				"key": string(make([]byte, _maxMetadataValueSize+1)),
			},
			expectError:    true,
			errorSubstring: "exceeds the maximum of 524288 bytes",
		},
		{
			name: "ValueExactlyMaxSize",
			metadata: map[string]string{
				"key": string(make([]byte, _maxMetadataValueSize)),
			},
			expectError: false,
		},
		{
			name: "MultipleValidationErrors",
			metadata: func() map[string]string {
				metadata := makeMetadataWithKeys(_maxMetadataKeys + 1)
				longKey := string(make([]byte, _maxMetadataKeyLength+1))
				metadata[longKey] = "value"
				return metadata
			}(),
			expectError:    true,
			errorSubstring: "exceeds the maximum of 32", // First validation error
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMetadata(tc.metadata)
			if tc.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errorSubstring)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConvertResponse(t *testing.T) {
	testCases := []struct {
		name         string
		input        *store.AssignedState
		expectedResp *types.ExecutorHeartbeatResponse
	}{
		{
			name:  "Nil input",
			input: nil,
			expectedResp: &types.ExecutorHeartbeatResponse{
				ShardAssignments: make(map[string]*types.ShardAssignment),
				MigrationMode:    types.MigrationModeONBOARDED,
			},
		},
		{
			name: "Empty input",
			input: &store.AssignedState{
				AssignedShards: make(map[string]*types.ShardAssignment),
			},
			expectedResp: &types.ExecutorHeartbeatResponse{
				ShardAssignments: make(map[string]*types.ShardAssignment),
				MigrationMode:    types.MigrationModeONBOARDED,
			},
		},
		{
			name: "Populated input",
			input: &store.AssignedState{
				AssignedShards: makeReadyAssignedShards("shard-1", "shard-2"),
			},
			expectedResp: &types.ExecutorHeartbeatResponse{
				ShardAssignments: makeReadyAssignedShards("shard-1", "shard-2"),
				MigrationMode:    types.MigrationModeONBOARDED,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// In Go, you can't initialize a map in a struct to nil directly,
			// so we handle the nil case for ShardAssignments separately for comparison.
			if tc.expectedResp.ShardAssignments == nil {
				tc.expectedResp.ShardAssignments = make(map[string]*types.ShardAssignment)
			}
			res := _convertResponse(tc.input, types.MigrationModeONBOARDED)

			// Ensure ShardAssignments is not nil for comparison purposes
			if res.ShardAssignments == nil {
				res.ShardAssignments = make(map[string]*types.ShardAssignment)
			}
			require.Equal(t, tc.expectedResp, res)
		})
	}
}

func TestFilterNewlyAssignedShardIDs(t *testing.T) {
	type testCase struct {
		name     string
		previous *store.HeartbeatState
		assigned *store.AssignedState
		expected []string
	}
	tests := []testCase{
		{
			name:     "nil previousHeartbeat returns all assigned",
			previous: nil,
			assigned: &store.AssignedState{
				AssignedShards: map[string]*types.ShardAssignment{
					"shard1": {},
					"shard2": {},
				},
			},
			expected: []string{"shard1", "shard2"},
		},
		{
			name: "no new assigned shards",
			previous: &store.HeartbeatState{
				ReportedShards: map[string]*types.ShardStatusReport{
					"shard1": {},
					"shard2": {},
				},
			},
			assigned: &store.AssignedState{
				AssignedShards: map[string]*types.ShardAssignment{
					"shard1": {},
					"shard2": {},
				},
			},
			expected: []string{},
		},
		{
			name: "some new assigned shards",
			previous: &store.HeartbeatState{
				ReportedShards: map[string]*types.ShardStatusReport{
					"shard1": {},
				},
			},
			assigned: &store.AssignedState{
				AssignedShards: map[string]*types.ShardAssignment{
					"shard1": {},
					"shard2": {},
					"shard3": {},
				},
			},
			expected: []string{"shard2", "shard3"},
		},
		{
			name: "empty assigned returns empty",
			previous: &store.HeartbeatState{
				ReportedShards: map[string]*types.ShardStatusReport{
					"shard1": {},
				},
			},
			assigned: &store.AssignedState{
				AssignedShards: map[string]*types.ShardAssignment{},
			},
			expected: []string{},
		},
		{
			name: "nil assignedState returns nil",
			previous: &store.HeartbeatState{
				ReportedShards: map[string]*types.ShardStatusReport{
					"shard1": {},
				},
			},
			assigned: nil,
			expected: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterNewlyAssignedShardIDs(tt.previous, tt.assigned)
			require.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestEmitShardAssignmentMetrics(t *testing.T) {
	heartbeatTime := time.Now().UTC()
	namespace := "test-namespace"
	shardID := "shard-1"
	emptyHeartbeatState := &store.HeartbeatState{ReportedShards: map[string]*types.ShardStatusReport{}}

	type expectHandoverMetric struct {
		Latency      time.Duration
		HandoverType types.HandoverType
	}

	type testCase struct {
		name              string
		previousHeartbeat *store.HeartbeatState
		assignedState     *store.AssignedState

		expectedDistributionLatency *time.Duration
		expectedHandoverLatencies   []*expectHandoverMetric
	}

	testCases := []testCase{
		{
			name:                        "no new assigned shards",
			previousHeartbeat:           &store.HeartbeatState{ReportedShards: map[string]*types.ShardStatusReport{shardID: {}}},
			assignedState:               &store.AssignedState{AssignedShards: makeReadyAssignedShards(shardID)},
			expectedDistributionLatency: nil,
			expectedHandoverLatencies:   nil,
		},
		{
			name:              "newly assigned shard with handover stats",
			previousHeartbeat: emptyHeartbeatState,
			assignedState: &store.AssignedState{
				AssignedShards: makeReadyAssignedShards(shardID),
				LastUpdated:    heartbeatTime.Add(-10 * time.Second),
				ShardHandoverStats: map[string]store.ShardHandoverStats{
					shardID: {
						PreviousExecutorLastHeartbeatTime: heartbeatTime.Add(-20 * time.Second),
						HandoverType:                      types.HandoverTypeGRACEFUL,
					},
				},
			},
			expectedDistributionLatency: common.Ptr(10 * time.Second),
			expectedHandoverLatencies:   []*expectHandoverMetric{{Latency: 20 * time.Second, HandoverType: types.HandoverTypeGRACEFUL}},
		},
		{
			name:              "one assigned shard with no handover stats",
			previousHeartbeat: emptyHeartbeatState,
			assignedState: &store.AssignedState{
				AssignedShards:     makeReadyAssignedShards(shardID),
				LastUpdated:        heartbeatTime.Add(-5 * time.Second),
				ShardHandoverStats: map[string]store.ShardHandoverStats{},
			},
			expectedDistributionLatency: common.Ptr(5 * time.Second),
			expectedHandoverLatencies:   nil,
		},
		{
			name:              "one assigned shard with nil handover stats",
			previousHeartbeat: emptyHeartbeatState,
			assignedState: &store.AssignedState{
				AssignedShards:     makeReadyAssignedShards(shardID),
				LastUpdated:        heartbeatTime.Add(-5 * time.Second),
				ShardHandoverStats: nil,
			},
			expectedDistributionLatency: common.Ptr(5 * time.Second),
			expectedHandoverLatencies:   nil,
		},
		{
			name:              "multiple newly assigned shards with handover stats",
			previousHeartbeat: emptyHeartbeatState,
			assignedState: &store.AssignedState{
				AssignedShards: makeReadyAssignedShards("shard-1", "shard-2"),
				LastUpdated:    heartbeatTime.Add(-15 * time.Second),
				ShardHandoverStats: map[string]store.ShardHandoverStats{
					"shard-1": {
						PreviousExecutorLastHeartbeatTime: heartbeatTime.Add(-25 * time.Second),
						HandoverType:                      types.HandoverTypeGRACEFUL,
					},
					"shard-2": {
						PreviousExecutorLastHeartbeatTime: heartbeatTime.Add(-30 * time.Second),
						HandoverType:                      types.HandoverTypeEMERGENCY,
					},
				},
			},
			expectedDistributionLatency: common.Ptr(15 * time.Second),
			expectedHandoverLatencies: []*expectHandoverMetric{
				{Latency: 30 * time.Second, HandoverType: types.HandoverTypeGRACEFUL},
				{Latency: 25 * time.Second, HandoverType: types.HandoverTypeEMERGENCY},
			},
		},
		{
			name:              "multiple newly assigned shards with some handover stats",
			previousHeartbeat: emptyHeartbeatState,
			assignedState: &store.AssignedState{
				AssignedShards: makeReadyAssignedShards("shard-1", "shard-2"),
				LastUpdated:    heartbeatTime.Add(-15 * time.Second),
				ShardHandoverStats: map[string]store.ShardHandoverStats{
					"shard-1": {
						PreviousExecutorLastHeartbeatTime: heartbeatTime.Add(-25 * time.Second),
						HandoverType:                      types.HandoverTypeGRACEFUL,
					},
				},
			},
			expectedDistributionLatency: common.Ptr(15 * time.Second),
			expectedHandoverLatencies:   []*expectHandoverMetric{{Latency: 25 * time.Second, HandoverType: types.HandoverTypeGRACEFUL}},
		},
		{
			name:              "multiple newly assigned shards without handover stats",
			previousHeartbeat: emptyHeartbeatState,
			assignedState: &store.AssignedState{
				AssignedShards: makeReadyAssignedShards("shard-1", "shard-2"),
				LastUpdated:    heartbeatTime.Add(-15 * time.Second),
			},
			expectedDistributionLatency: common.Ptr(15 * time.Second),
			expectedHandoverLatencies:   nil,
		},
		{
			name:              "nil handover stats with new assigned shard",
			previousHeartbeat: emptyHeartbeatState,
			assignedState: &store.AssignedState{
				AssignedShards:     makeReadyAssignedShards(shardID),
				LastUpdated:        heartbeatTime.Add(-5 * time.Second),
				ShardHandoverStats: nil,
			},
			expectedDistributionLatency: common.Ptr(5 * time.Second),
			expectedHandoverLatencies:   nil,
		},
		{
			name: "newly assigned shard with previous heartbeat containing reported shards",
			previousHeartbeat: &store.HeartbeatState{
				ReportedShards: map[string]*types.ShardStatusReport{
					"shard-2": {},
				},
			},
			assignedState: &store.AssignedState{
				AssignedShards: makeReadyAssignedShards("shard-1", "shard-2"),
				LastUpdated:    heartbeatTime.Add(-8 * time.Second),
				ShardHandoverStats: map[string]store.ShardHandoverStats{
					"shard-1": {
						PreviousExecutorLastHeartbeatTime: heartbeatTime.Add(-18 * time.Second),
						HandoverType:                      types.HandoverTypeGRACEFUL,
					},
				},
			},
			expectedDistributionLatency: common.Ptr(8 * time.Second),
			expectedHandoverLatencies:   []*expectHandoverMetric{{Latency: 18 * time.Second, HandoverType: types.HandoverTypeGRACEFUL}},
		},
		{
			name: "multiple new assigned shards, previous heartbeat contains some",
			previousHeartbeat: &store.HeartbeatState{
				ReportedShards: map[string]*types.ShardStatusReport{
					"shard-2": {},
				},
			},
			assignedState: &store.AssignedState{
				AssignedShards: map[string]*types.ShardAssignment{
					"shard-1": {Status: types.AssignmentStatusREADY},
					"shard-2": {Status: types.AssignmentStatusREADY},
					"shard-3": {Status: types.AssignmentStatusREADY},
				},
				LastUpdated: heartbeatTime.Add(-12 * time.Second),
				ShardHandoverStats: map[string]store.ShardHandoverStats{
					"shard-1": {
						PreviousExecutorLastHeartbeatTime: heartbeatTime.Add(-22 * time.Second),
						HandoverType:                      types.HandoverTypeGRACEFUL,
					},
					"shard-3": {
						PreviousExecutorLastHeartbeatTime: heartbeatTime.Add(-30 * time.Second),
						HandoverType:                      types.HandoverTypeEMERGENCY,
					},
				},
			},
			expectedDistributionLatency: common.Ptr(12 * time.Second),
			expectedHandoverLatencies: []*expectHandoverMetric{
				{Latency: 22 * time.Second, HandoverType: types.HandoverTypeGRACEFUL},
				{Latency: 30 * time.Second, HandoverType: types.HandoverTypeEMERGENCY},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			metricsClient := &metricmocks.Client{}
			metricsScope := &metricmocks.Scope{}

			if tc.expectedDistributionLatency != nil {
				metricsClient.On("Scope", metrics.ShardDistributorHeartbeatScope).Return(metricsScope).Once()
				metricsScope.On("Tagged", metrics.NamespaceTag(namespace)).Return(metricsScope).Once()
				metricsScope.On("RecordHistogramDuration", metrics.ShardDistributorShardAssignmentDistributionLatency, *tc.expectedDistributionLatency).Once()
			}

			if tc.expectedHandoverLatencies != nil {
				for _, expected := range tc.expectedHandoverLatencies {
					metricsScope.On("Tagged", metrics.HandoverTypeTag(expected.HandoverType.String())).Return(metricsScope)
					metricsScope.On("RecordHistogramDuration", metrics.ShardDistributorShardHandoverLatency, expected.Latency).Once()
				}
			}

			exec := &executor{metricsClient: metricsClient, logger: testlogger.New(t)}
			exec.emitShardAssignmentMetrics(namespace, heartbeatTime, tc.previousHeartbeat, tc.assignedState)

			metricsClient.AssertExpectations(t)
			metricsScope.AssertExpectations(t)
		})
	}
}

func newTestExecutorHandler(t *testing.T, storage store.Store, timeSource clock.TimeSource) Executor {
	t.Helper()
	return NewExecutorHandler(
		testlogger.New(t),
		storage,
		timeSource,
		newTestShardDistributorConfig(config.LoadBalancingModeNAIVE),
		metrics.NoopClient,
	)
}

// makeReadyAssignedShards is a helper function to create a map of shard assignments with READY status.
func makeReadyAssignedShards(shardIDs ...string) map[string]*types.ShardAssignment {
	return makeAssignedShards(types.AssignmentStatusREADY, shardIDs...)
}

// makeAssignedShards is a helper function to create a map of shard assignments with the given status.
func makeAssignedShards(status types.AssignmentStatus, shardIDs ...string) map[string]*types.ShardAssignment {
	assignedShards := make(map[string]*types.ShardAssignment)
	for _, shardID := range shardIDs {
		assignedShards[shardID] = &types.ShardAssignment{Status: status}
	}
	return assignedShards
}
