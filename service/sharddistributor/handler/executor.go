package handler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/loadbalancer"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

const (
	_maxMetadataKeys      = 32
	_maxMetadataKeyLength = 128
	_maxMetadataValueSize = 512 * 1024 // 512KB
)

type executor struct {
	logger        log.Logger
	timeSource    clock.TimeSource
	storage       store.Store
	sdConfig      *config.Config
	metricsClient metrics.Client
}

func NewExecutorHandler(
	logger log.Logger,
	storage store.Store,
	timeSource clock.TimeSource,
	cfg *config.Config,
	metricsClient metrics.Client,
) Executor {
	return &executor{
		logger:        logger,
		timeSource:    timeSource,
		storage:       storage,
		sdConfig:      cfg,
		metricsClient: metricsClient,
	}
}

func (h *executor) Heartbeat(ctx context.Context, request *types.ExecutorHeartbeatRequest) (*types.ExecutorHeartbeatResponse, error) {
	executorState, err := h.storage.GetExecutorState(ctx, request.Namespace, request.ExecutorID)
	// We ignore Executor not found errors, since it just means that this executor heartbeat the first time.
	if err != nil && !errors.Is(err, store.ErrExecutorNotFound) {
		return nil, &types.InternalServiceError{Message: fmt.Sprintf("failed to get heartbeat: %v", err)}
	}

	heartbeatTime := h.timeSource.Now().UTC()

	newHeartbeat := store.HeartbeatState{
		LastHeartbeat:  heartbeatTime,
		Status:         request.Status,
		ReportedShards: request.ShardStatusReports,
		Metadata:       request.GetMetadata(),
	}

	if err := validateMetadata(newHeartbeat.Metadata); err != nil {
		return nil, types.BadRequestError{Message: fmt.Sprintf("invalid metadata: %s", err)}
	}

	err = h.storage.RecordHeartbeat(ctx, request.Namespace, request.ExecutorID, newHeartbeat)
	if err != nil {
		return nil, &types.InternalServiceError{Message: fmt.Sprintf("failed to record heartbeat: %v", err)}
	}

	h.recordShardStatistics(ctx, request, executorState.Assignment, executorState.Statistics, heartbeatTime)

	// emit shard assignment metrics only if shards are assigned in the background
	// shard assignment in heartbeat doesn't involve any assignment changes happening in the background
	// thus there was no shard handover and no assignment distribution latency
	// to measure, so don't need to emit metrics in that case
	h.emitShardAssignmentMetrics(request.Namespace, heartbeatTime, executorState.Heartbeat, executorState.Assignment)

	return _convertResponse(executorState.Assignment, types.MigrationModeONBOARDED), nil
}

func (h *executor) recordShardStatistics(
	ctx context.Context,
	request *types.ExecutorHeartbeatRequest,
	assignedState *store.AssignedState,
	previousStats map[string]store.ShardStatistics,
	now time.Time,
) {
	updatedStats, shouldWrite, err := loadbalancer.PrepareShardStatistics(
		h.sdConfig,
		request.Namespace,
		request.ExecutorID,
		request.ShardStatusReports,
		assignedState,
		previousStats,
		now,
		h.logger,
	)
	if err != nil {
		h.logger.Warn("failed to prepare shard statistics",
			tag.Error(err),
			tag.ShardNamespace(request.Namespace),
			tag.ShardExecutor(request.ExecutorID),
		)
		return
	}
	if !shouldWrite {
		return
	}

	err = h.storage.RecordShardStatistics(
		ctx,
		request.Namespace,
		request.ExecutorID,
		assignedState.ModRevision,
		updatedStats,
	)
	if err == nil || errors.Is(err, store.ErrVersionConflict) {
		return
	}

	h.logger.Warn("failed to record shard statistics",
		tag.Error(err),
		tag.ShardNamespace(request.Namespace),
		tag.ShardExecutor(request.ExecutorID),
	)
}

// emitShardAssignmentMetrics emits the following metrics for newly assigned shards:
// - ShardAssignmentDistributionLatency: time taken since the shard was assigned to heartbeat time
// - ShardHandoverLatency: time taken since the previous executor's last heartbeat to heartbeat time
func (h *executor) emitShardAssignmentMetrics(namespace string, heartbeatTime time.Time, previousHeartbeat *store.HeartbeatState, assignedState *store.AssignedState) {
	// find newly assigned shards, if there are none, no handovers happened
	newAssignedShardIDs := filterNewlyAssignedShardIDs(previousHeartbeat, assignedState)
	if len(newAssignedShardIDs) == 0 {
		// no need to emit ShardDistributorShardAssignmentDistributionLatency due to no handovers
		return
	}

	metricsScope := h.metricsClient.Scope(metrics.ShardDistributorHeartbeatScope).
		Tagged(metrics.NamespaceTag(namespace))

	distributionLatency := heartbeatTime.Sub(assignedState.LastUpdated)
	metricsScope.RecordHistogramDuration(metrics.ShardDistributorShardAssignmentDistributionLatency, distributionLatency)

	for _, shardID := range newAssignedShardIDs {
		handoverStats, ok := assignedState.ShardHandoverStats[shardID]
		if !ok {
			// no handover stats for this shard, means it was never handed over before
			// so no handover latency metric to emit
			continue
		}

		handoverLatency := heartbeatTime.Sub(handoverStats.PreviousExecutorLastHeartbeatTime)
		metricsScope.Tagged(metrics.HandoverTypeTag(handoverStats.HandoverType.String())).
			RecordHistogramDuration(metrics.ShardDistributorShardHandoverLatency, handoverLatency)

	}
}

func _convertResponse(shards *store.AssignedState, mode types.MigrationMode) *types.ExecutorHeartbeatResponse {
	res := &types.ExecutorHeartbeatResponse{}
	res.MigrationMode = mode
	if shards == nil {
		return res
	}
	res.ShardAssignments = shards.AssignedShards
	return res
}

func validateMetadata(metadata map[string]string) error {
	if len(metadata) > _maxMetadataKeys {
		return fmt.Errorf("metadata has %d keys, which exceeds the maximum of %d", len(metadata), _maxMetadataKeys)
	}

	for key, value := range metadata {
		if len(key) > _maxMetadataKeyLength {
			return fmt.Errorf("metadata key %q has length %d, which exceeds the maximum of %d", key, len(key), _maxMetadataKeyLength)
		}

		if len(value) > _maxMetadataValueSize {
			return fmt.Errorf("metadata value for key %q has size %d bytes, which exceeds the maximum of %d bytes", key, len(value), _maxMetadataValueSize)
		}
	}

	return nil
}

func filterNewlyAssignedShardIDs(previousHeartbeat *store.HeartbeatState, assignedState *store.AssignedState) []string {
	// if assignedState is nil, no shards are assigned
	if assignedState == nil {
		return nil
	}

	var newAssignedShardIDs []string
	for assignedShardID := range assignedState.AssignedShards {
		if previousHeartbeat == nil || !shardInReportedShards(previousHeartbeat.ReportedShards, assignedShardID) {
			newAssignedShardIDs = append(newAssignedShardIDs, assignedShardID)
		}
	}

	return newAssignedShardIDs
}

func shardInReportedShards(reportedShards map[string]*types.ShardStatusReport, shardID string) bool {
	_, ok := reportedShards[shardID]
	return ok
}
