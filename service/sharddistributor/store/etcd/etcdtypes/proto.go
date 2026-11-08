package etcdtypes

import (
	"fmt"
	"time"

	storev1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/store/v1"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// The mappers below convert between the store types and the protobuf values stored in
// etcd. Enum values without a stored equivalent return an error in both directions, so
// a value added on one side fails loudly instead of being stored or read as INVALID.
// Timestamps are stored with millisecond precision, and the smoothed load as a float32.

// ExecutorStatusToProto converts an executor status to its stored value.
func ExecutorStatusToProto(status types.ExecutorStatus) (*storev1.ExecutorStatusValue, error) {
	storedStatus, err := executorStatusToProto(status)
	if err != nil {
		return nil, err
	}
	return &storev1.ExecutorStatusValue{Status: storedStatus}, nil
}

// ExecutorStatusFromProto converts a stored executor status value to an executor status.
func ExecutorStatusFromProto(value *storev1.ExecutorStatusValue) (types.ExecutorStatus, error) {
	return executorStatusFromProto(value.GetStatus())
}

// ReportedShardsToProto converts the shards an executor reports to their stored value.
func ReportedShardsToProto(reportedShards map[string]*types.ShardStatusReport) (*storev1.ReportedShardsValue, error) {
	shards := make(map[string]*storev1.ReportedShard, len(reportedShards))
	for shardID, report := range reportedShards {
		status, err := shardStatusToProto(report.GetStatus())
		if err != nil {
			return nil, fmt.Errorf("shard %q: %w", shardID, err)
		}
		shards[shardID] = &storev1.ReportedShard{
			Status: status,
			Load:   report.GetShardLoad(),
		}
	}
	return &storev1.ReportedShardsValue{Shards: shards}, nil
}

// ReportedShardsFromProto converts a stored reported shards value to the shards an
// executor reports.
func ReportedShardsFromProto(value *storev1.ReportedShardsValue) (map[string]*types.ShardStatusReport, error) {
	reportedShards := make(map[string]*types.ShardStatusReport, len(value.GetShards()))
	for shardID, shard := range value.GetShards() {
		status, err := shardStatusFromProto(shard.GetStatus())
		if err != nil {
			return nil, fmt.Errorf("shard %q: %w", shardID, err)
		}
		reportedShards[shardID] = &types.ShardStatusReport{
			Status:    status,
			ShardLoad: shard.GetLoad(),
		}
	}
	return reportedShards, nil
}

// AssignedStateToProto converts an executor's assigned state to its stored value. The
// mod revision is not stored, because it comes from the etcd key.
func AssignedStateToProto(assignedState *store.AssignedState) (*storev1.AssignedStateValue, error) {
	shards := make(map[string]*storev1.AssignedShard, len(assignedState.AssignedShards))
	for shardID, assignment := range assignedState.AssignedShards {
		status, err := assignmentStatusToProto(assignment.GetStatus())
		if err != nil {
			return nil, fmt.Errorf("shard %q: %w", shardID, err)
		}
		shards[shardID] = &storev1.AssignedShard{Status: status}
	}

	handovers := make(map[string]*storev1.ShardHandover, len(assignedState.ShardHandoverStats))
	for shardID, handover := range assignedState.ShardHandoverStats {
		handoverType, err := handoverTypeToProto(handover.HandoverType)
		if err != nil {
			return nil, fmt.Errorf("shard %q: %w", shardID, err)
		}
		handovers[shardID] = &storev1.ShardHandover{
			PreviousExecutorLastHeartbeatUnixMillis: timeToUnixMillis(handover.PreviousExecutorLastHeartbeatTime),
			Type:                                    handoverType,
		}
	}

	assignedStateValue := &storev1.AssignedStateValue{
		Shards:                shards,
		Handovers:             handovers,
		LastUpdatedUnixMillis: timeToUnixMillis(assignedState.LastUpdated),
	}
	return assignedStateValue, nil
}

// AssignedStateFromProto converts a stored assigned state value to an executor's
// assigned state. The caller sets the mod revision from the etcd key.
func AssignedStateFromProto(value *storev1.AssignedStateValue) (*store.AssignedState, error) {
	assignedShards := make(map[string]*types.ShardAssignment, len(value.GetShards()))
	for shardID, shard := range value.GetShards() {
		status, err := assignmentStatusFromProto(shard.GetStatus())
		if err != nil {
			return nil, fmt.Errorf("shard %q: %w", shardID, err)
		}
		assignedShards[shardID] = &types.ShardAssignment{Status: status}
	}

	handoverStats := make(map[string]store.ShardHandoverStats, len(value.GetHandovers()))
	for shardID, handover := range value.GetHandovers() {
		handoverType, err := handoverTypeFromProto(handover.GetType())
		if err != nil {
			return nil, fmt.Errorf("shard %q: %w", shardID, err)
		}
		handoverStats[shardID] = store.ShardHandoverStats{
			PreviousExecutorLastHeartbeatTime: unixMillisToTime(handover.GetPreviousExecutorLastHeartbeatUnixMillis()),
			HandoverType:                      handoverType,
		}
	}

	assignedState := &store.AssignedState{
		AssignedShards:     assignedShards,
		ShardHandoverStats: handoverStats,
		LastUpdated:        unixMillisToTime(value.GetLastUpdatedUnixMillis()),
	}
	return assignedState, nil
}

// ShardStatisticsToProto converts an executor's shard statistics to their stored value.
func ShardStatisticsToProto(statistics map[string]store.ShardStatistics) *storev1.ShardStatisticsValue {
	shards := make(map[string]*storev1.ShardStatistics, len(statistics))
	for shardID, statistic := range statistics {
		shards[shardID] = &storev1.ShardStatistics{
			SmoothedLoad:         float32(statistic.SmoothedLoad),
			LastUpdateUnixMillis: timeToUnixMillis(statistic.LastUpdateTime),
			LastMoveUnixMillis:   timeToUnixMillis(statistic.LastMoveTime),
		}
	}
	return &storev1.ShardStatisticsValue{Shards: shards}
}

// ShardStatisticsFromProto converts a stored shard statistics value to an executor's
// shard statistics.
func ShardStatisticsFromProto(value *storev1.ShardStatisticsValue) map[string]store.ShardStatistics {
	statistics := make(map[string]store.ShardStatistics, len(value.GetShards()))
	for shardID, statistic := range value.GetShards() {
		statistics[shardID] = store.ShardStatistics{
			SmoothedLoad:   float64(statistic.GetSmoothedLoad()),
			LastUpdateTime: unixMillisToTime(statistic.GetLastUpdateUnixMillis()),
			LastMoveTime:   unixMillisToTime(statistic.GetLastMoveUnixMillis()),
		}
	}
	return statistics
}

func executorStatusToProto(status types.ExecutorStatus) (storev1.ExecutorStatus, error) {
	switch status {
	case types.ExecutorStatusINVALID:
		return storev1.ExecutorStatus_EXECUTOR_STATUS_INVALID, nil
	case types.ExecutorStatusACTIVE:
		return storev1.ExecutorStatus_EXECUTOR_STATUS_ACTIVE, nil
	case types.ExecutorStatusDRAINING:
		return storev1.ExecutorStatus_EXECUTOR_STATUS_DRAINING, nil
	case types.ExecutorStatusDRAINED:
		return storev1.ExecutorStatus_EXECUTOR_STATUS_DRAINED, nil
	default:
		return storev1.ExecutorStatus_EXECUTOR_STATUS_INVALID, fmt.Errorf("unknown executor status %v", status)
	}
}

func executorStatusFromProto(status storev1.ExecutorStatus) (types.ExecutorStatus, error) {
	switch status {
	case storev1.ExecutorStatus_EXECUTOR_STATUS_INVALID:
		return types.ExecutorStatusINVALID, nil
	case storev1.ExecutorStatus_EXECUTOR_STATUS_ACTIVE:
		return types.ExecutorStatusACTIVE, nil
	case storev1.ExecutorStatus_EXECUTOR_STATUS_DRAINING:
		return types.ExecutorStatusDRAINING, nil
	case storev1.ExecutorStatus_EXECUTOR_STATUS_DRAINED:
		return types.ExecutorStatusDRAINED, nil
	default:
		return types.ExecutorStatusINVALID, fmt.Errorf("unknown stored executor status %v", status)
	}
}

func shardStatusToProto(status types.ShardStatus) (storev1.ShardStatus, error) {
	switch status {
	case types.ShardStatusINVALID:
		return storev1.ShardStatus_SHARD_STATUS_INVALID, nil
	case types.ShardStatusREADY:
		return storev1.ShardStatus_SHARD_STATUS_READY, nil
	case types.ShardStatusDONE:
		return storev1.ShardStatus_SHARD_STATUS_DONE, nil
	default:
		return storev1.ShardStatus_SHARD_STATUS_INVALID, fmt.Errorf("unknown shard status %v", status)
	}
}

func shardStatusFromProto(status storev1.ShardStatus) (types.ShardStatus, error) {
	switch status {
	case storev1.ShardStatus_SHARD_STATUS_INVALID:
		return types.ShardStatusINVALID, nil
	case storev1.ShardStatus_SHARD_STATUS_READY:
		return types.ShardStatusREADY, nil
	case storev1.ShardStatus_SHARD_STATUS_DONE:
		return types.ShardStatusDONE, nil
	default:
		return types.ShardStatusINVALID, fmt.Errorf("unknown stored shard status %v", status)
	}
}

func assignmentStatusToProto(status types.AssignmentStatus) (storev1.AssignmentStatus, error) {
	switch status {
	case types.AssignmentStatusINVALID:
		return storev1.AssignmentStatus_ASSIGNMENT_STATUS_INVALID, nil
	case types.AssignmentStatusREADY:
		return storev1.AssignmentStatus_ASSIGNMENT_STATUS_READY, nil
	default:
		return storev1.AssignmentStatus_ASSIGNMENT_STATUS_INVALID, fmt.Errorf("unknown assignment status %v", status)
	}
}

func assignmentStatusFromProto(status storev1.AssignmentStatus) (types.AssignmentStatus, error) {
	switch status {
	case storev1.AssignmentStatus_ASSIGNMENT_STATUS_INVALID:
		return types.AssignmentStatusINVALID, nil
	case storev1.AssignmentStatus_ASSIGNMENT_STATUS_READY:
		return types.AssignmentStatusREADY, nil
	default:
		return types.AssignmentStatusINVALID, fmt.Errorf("unknown stored assignment status %v", status)
	}
}

func handoverTypeToProto(handoverType types.HandoverType) (storev1.HandoverType, error) {
	switch handoverType {
	case types.HandoverTypeINVALID:
		return storev1.HandoverType_HANDOVER_TYPE_INVALID, nil
	case types.HandoverTypeGRACEFUL:
		return storev1.HandoverType_HANDOVER_TYPE_GRACEFUL, nil
	case types.HandoverTypeEMERGENCY:
		return storev1.HandoverType_HANDOVER_TYPE_EMERGENCY, nil
	default:
		return storev1.HandoverType_HANDOVER_TYPE_INVALID, fmt.Errorf("unknown handover type %v", handoverType)
	}
}

func handoverTypeFromProto(handoverType storev1.HandoverType) (types.HandoverType, error) {
	switch handoverType {
	case storev1.HandoverType_HANDOVER_TYPE_INVALID:
		return types.HandoverTypeINVALID, nil
	case storev1.HandoverType_HANDOVER_TYPE_GRACEFUL:
		return types.HandoverTypeGRACEFUL, nil
	case storev1.HandoverType_HANDOVER_TYPE_EMERGENCY:
		return types.HandoverTypeEMERGENCY, nil
	default:
		return types.HandoverTypeINVALID, fmt.Errorf("unknown stored handover type %v", handoverType)
	}
}

// timeToUnixMillis stores the zero time as 0, so unset timestamps take no space.
func timeToUnixMillis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func unixMillisToTime(unixMillis int64) time.Time {
	if unixMillis == 0 {
		return time.Time{}
	}
	return time.UnixMilli(unixMillis).UTC()
}
