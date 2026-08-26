package metered

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber-go/tally"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

const (
	_testNamespace  = "test_namespace"
	_testExecutorID = "test_executorID"
)

func TestMeteredStore_GetExecutorState(t *testing.T) {
	executorState := store.ExecutorState{
		Heartbeat: &store.HeartbeatState{
			LastHeartbeat: time.Now().UTC(),
		},
		Assignment: &store.AssignedState{
			LastUpdated: time.Now().UTC(),
		},
		Statistics: map[string]store.ShardStatistics{
			"shard-1": {SmoothedLoad: 12.3},
		},
	}

	tests := []struct {
		name  string
		error error
	}{
		{
			name:  "Success",
			error: nil,
		},
		{
			name:  "NotFound",
			error: store.ErrExecutorNotFound,
		},
		{
			name:  "Failure",
			error: &types.InternalServiceError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			testScope := tally.NewTestScope("test", nil)
			metricsClient := metrics.NewClient(testScope, metrics.ShardDistributor, metrics.MigrationConfig{})
			timeSource := clock.NewMockedTimeSource()
			mockHandler := store.NewMockStore(ctrl)

			mockHandler.EXPECT().GetExecutorState(gomock.Any(), _testNamespace, _testExecutorID).Do(func(ctx context.Context, namespace string, executorID string) {
				timeSource.Advance(time.Second)
			}).Return(executorState, tt.error)

			mockLogger := log.NewMockLogger(ctrl)
			mockLogger.EXPECT().Helper().Return(mockLogger).AnyTimes()

			wrapped := NewStore(mockHandler, metricsClient, mockLogger, timeSource).(*meteredStore)

			gotExecutorState, err := wrapped.GetExecutorState(context.Background(), _testNamespace, _testExecutorID)

			assert.Equal(t, executorState, gotExecutorState)
			assert.Equal(t, tt.error, err)

			// check that the metrics were emitted for this method
			requestCounterName := "test.shard_distributor_store_requests_per_namespace+namespace=test_namespace,operation=StoreGetExecutorState"
			assert.Contains(t, testScope.Snapshot().Counters(), requestCounterName)
			requestCounter := testScope.Snapshot().Counters()[requestCounterName]
			assert.Equal(t, int64(1), requestCounter.Value())

			latencyHistogramName := "test.shard_distributor_store_latency_histogram_per_namespace+namespace=test_namespace,operation=StoreGetExecutorState"
			allHistograms := testScope.Snapshot().Histograms()
			assert.Contains(t, allHistograms, latencyHistogramName)
		})
	}
}

func TestMeteredStore_RecordShardStatisticsErrorMetrics(t *testing.T) {
	assignmentChangedError := fmt.Errorf("%w: executor assignment changed", store.ErrVersionConflict)

	tests := []struct {
		name                string
		storeError          error
		expectSkippedMetric bool
		expectFailureMetric bool
	}{
		{
			name:                "AssignmentChanged",
			storeError:          assignmentChangedError,
			expectSkippedMetric: true,
		},
		{
			name:                "StoreFailure",
			storeError:          assert.AnError,
			expectFailureMetric: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			assignmentModRevision := int64(1)
			statistics := map[string]store.ShardStatistics{
				"shard-1": {SmoothedLoad: 12.3},
			}

			testScope := tally.NewTestScope("test", nil)
			metricsClient := metrics.NewClient(testScope, metrics.ShardDistributor, metrics.MigrationConfig{})
			timeSource := clock.NewMockedTimeSource()
			mockStore := store.NewMockStore(ctrl)
			mockStore.EXPECT().RecordShardStatistics(
				gomock.Any(),
				_testNamespace,
				_testExecutorID,
				assignmentModRevision,
				statistics,
			).Return(tt.storeError)

			wrapped := NewStore(mockStore, metricsClient, log.NewNoop(), timeSource)

			err := wrapped.RecordShardStatistics(
				context.Background(),
				_testNamespace,
				_testExecutorID,
				assignmentModRevision,
				statistics,
			)
			assert.Equal(t, tt.storeError, err)

			counters := testScope.Snapshot().Counters()
			skippedMetricName := "test.shard_distributor_store_shard_statistics_skipped+namespace=test_namespace,operation=StoreRecordShardStatistics"
			failureMetricName := "test.shard_distributor_store_failures_per_namespace+namespace=test_namespace,operation=StoreRecordShardStatistics"
			assert.Equal(t, tt.expectSkippedMetric, counters[skippedMetricName] != nil)
			assert.Equal(t, tt.expectFailureMetric, counters[failureMetricName] != nil)
		})
	}
}
