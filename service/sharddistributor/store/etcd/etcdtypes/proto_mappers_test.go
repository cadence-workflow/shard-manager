package etcdtypes

import (
	"reflect"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storev1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/store/v1"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// TestProtoMappersCoverAllFields fails when a field is added to a stored type, so the
// protobuf schema and mappers are updated along with it.
func TestProtoMappersCoverAllFields(t *testing.T) {
	tests := map[string]struct {
		storedType       reflect.Type
		mappedFieldCount int
	}{
		"store.AssignedState": {
			storedType: reflect.TypeOf(store.AssignedState{}),
			// ModRevision comes from the etcd key and is not stored.
			mappedFieldCount: 4,
		},
		"store.ShardHandoverStats": {
			storedType:       reflect.TypeOf(store.ShardHandoverStats{}),
			mappedFieldCount: 2,
		},
		"store.ShardStatistics": {
			storedType:       reflect.TypeOf(store.ShardStatistics{}),
			mappedFieldCount: 3,
		},
		"types.ShardStatusReport": {
			storedType:       reflect.TypeOf(types.ShardStatusReport{}),
			mappedFieldCount: 2,
		},
		"types.ShardAssignment": {
			storedType:       reflect.TypeOf(types.ShardAssignment{}),
			mappedFieldCount: 1,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.mappedFieldCount, tt.storedType.NumField(),
				"%s changed; update the protobuf schema and mappers in proto_mappers.go, then this count", name)
		})
	}
}

func TestExecutorStatusProtoRoundTrip(t *testing.T) {
	for _, status := range types.ExecutorStatusValues() {
		t.Run(status.String(), func(t *testing.T) {
			value, err := ExecutorStatusToProto(status)
			require.NoError(t, err)

			decoded := &storev1.ExecutorStatusValue{}
			roundTripBytes(t, value, decoded)

			got, err := ExecutorStatusFromProto(decoded)
			require.NoError(t, err)
			assert.Equal(t, status, got)
		})
	}
}

func TestShardStatusProtoRoundTrip(t *testing.T) {
	for _, status := range types.ShardStatusValues() {
		t.Run(status.String(), func(t *testing.T) {
			storedStatus, err := shardStatusToProto(status)
			require.NoError(t, err)

			got, err := shardStatusFromProto(storedStatus)
			require.NoError(t, err)
			assert.Equal(t, status, got)
		})
	}
}

func TestAssignmentStatusProtoRoundTrip(t *testing.T) {
	for _, status := range types.AssignmentStatusValues() {
		t.Run(status.String(), func(t *testing.T) {
			storedStatus, err := assignmentStatusToProto(status)
			require.NoError(t, err)

			got, err := assignmentStatusFromProto(storedStatus)
			require.NoError(t, err)
			assert.Equal(t, status, got)
		})
	}
}

func TestHandoverTypeProtoRoundTrip(t *testing.T) {
	for _, handoverType := range types.HandoverTypeValues() {
		t.Run(handoverType.String(), func(t *testing.T) {
			storedType, err := handoverTypeToProto(handoverType)
			require.NoError(t, err)

			got, err := handoverTypeFromProto(storedType)
			require.NoError(t, err)
			assert.Equal(t, handoverType, got)
		})
	}
}

func TestProtoMappersRejectUnknownEnums(t *testing.T) {
	unknown := 99

	tests := map[string]struct {
		mapper  func() error
		wantErr string
	}{
		"executor status to proto": {
			mapper: func() error {
				_, err := ExecutorStatusToProto(types.ExecutorStatus(unknown))
				return err
			},
			wantErr: "unknown executor status",
		},
		"executor status from proto": {
			mapper: func() error {
				_, err := ExecutorStatusFromProto(&storev1.ExecutorStatusValue{Status: storev1.ExecutorStatus(unknown)})
				return err
			},
			wantErr: "unknown stored executor status",
		},
		"reported shards to proto": {
			mapper: func() error {
				_, err := ReportedShardsToProto(map[string]*types.ShardStatusReport{
					"shard-1": {Status: types.ShardStatus(unknown)},
				})
				return err
			},
			wantErr: `shard "shard-1": unknown shard status`,
		},
		"reported shards from proto": {
			mapper: func() error {
				_, err := ReportedShardsFromProto(&storev1.ReportedShardsValue{
					Shards: map[string]*storev1.ReportedShard{"shard-1": {Status: storev1.ShardStatus(unknown)}},
				})
				return err
			},
			wantErr: `shard "shard-1": unknown stored shard status`,
		},
		"assignment status to proto": {
			mapper: func() error {
				_, err := AssignedStateToProto(&store.AssignedState{
					AssignedShards: map[string]*types.ShardAssignment{"shard-1": {Status: types.AssignmentStatus(unknown)}},
				})
				return err
			},
			wantErr: `shard "shard-1": unknown assignment status`,
		},
		"assignment status from proto": {
			mapper: func() error {
				_, err := AssignedStateFromProto(&storev1.AssignedStateValue{
					Shards: map[string]*storev1.AssignedShard{"shard-1": {Status: storev1.AssignmentStatus(unknown)}},
				})
				return err
			},
			wantErr: `shard "shard-1": unknown stored assignment status`,
		},
		"handover type to proto": {
			mapper: func() error {
				_, err := AssignedStateToProto(&store.AssignedState{
					ShardHandoverStats: map[string]store.ShardHandoverStats{"shard-1": {HandoverType: types.HandoverType(unknown)}},
				})
				return err
			},
			wantErr: `shard "shard-1": unknown handover type`,
		},
		"handover type from proto": {
			mapper: func() error {
				_, err := AssignedStateFromProto(&storev1.AssignedStateValue{
					Handovers: map[string]*storev1.ShardHandover{"shard-1": {Type: storev1.HandoverType(unknown)}},
				})
				return err
			},
			wantErr: `shard "shard-1": unknown stored handover type`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := tt.mapper()
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestReportedShardsProtoRoundTrip(t *testing.T) {
	// float32 represents this load exactly, so it finishes the round trip unchanged.
	readyShardLoad := 412.5

	reportedShards := map[string]*types.ShardStatusReport{
		"shard-1": {Status: types.ShardStatusREADY, ShardLoad: readyShardLoad},
		"shard-2": {Status: types.ShardStatusDONE, ShardLoad: 0},
	}

	value, err := ReportedShardsToProto(reportedShards)
	require.NoError(t, err)

	decoded := &storev1.ReportedShardsValue{}
	roundTripBytes(t, value, decoded)

	got, err := ReportedShardsFromProto(decoded)
	require.NoError(t, err)
	assert.Equal(t, reportedShards, got)
}

func TestAssignedStateProtoRoundTrip(t *testing.T) {
	lastUpdated := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC).Add(123 * time.Millisecond)
	previousExecutorLastHeartbeat := time.Date(2026, 11, 8, 11, 59, 58, 0, time.UTC).Add(456 * time.Millisecond)

	tests := map[string]struct {
		assignedState *store.AssignedState
	}{
		// The decoded protobuf message holds nil maps here, and AssignedStateFromProto
		// must still return empty maps.
		"empty": {
			assignedState: &store.AssignedState{
				AssignedShards:     map[string]*types.ShardAssignment{},
				ShardHandoverStats: map[string]store.ShardHandoverStats{},
			},
		},
		"shards and handovers": {
			assignedState: &store.AssignedState{
				AssignedShards: map[string]*types.ShardAssignment{
					"shard-1": {Status: types.AssignmentStatusREADY},
					"shard-2": {Status: types.AssignmentStatusREADY},
				},
				ShardHandoverStats: map[string]store.ShardHandoverStats{
					"shard-1": {
						PreviousExecutorLastHeartbeatTime: previousExecutorLastHeartbeat,
						HandoverType:                      types.HandoverTypeEMERGENCY,
					},
				},
				LastUpdated: lastUpdated,
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			value, err := AssignedStateToProto(tt.assignedState)
			require.NoError(t, err)

			decoded := &storev1.AssignedStateValue{}
			roundTripBytes(t, value, decoded)

			got, err := AssignedStateFromProto(decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.assignedState, got)
		})
	}
}

func TestShardStatisticsProtoRoundTrip(t *testing.T) {
	// float32 represents these loads exactly, so they finish the round trip unchanged.
	movedShardLoad := 412.5
	neverMovedShardLoad := 3.25

	lastUpdate := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC).Add(789 * time.Millisecond)
	lastMove := time.Date(2026, 11, 7, 3, 15, 0, 0, time.UTC)

	statistics := map[string]store.ShardStatistics{
		"moved-shard": {
			SmoothedLoad:   movedShardLoad,
			LastUpdateTime: lastUpdate,
			LastMoveTime:   lastMove,
		},
		"never-moved-shard": {
			SmoothedLoad:   neverMovedShardLoad,
			LastUpdateTime: lastUpdate,
		},
	}

	value := ShardStatisticsToProto(statistics)

	decoded := &storev1.ShardStatisticsValue{}
	roundTripBytes(t, value, decoded)

	got := ShardStatisticsFromProto(decoded)
	assert.Equal(t, statistics, got)
}

func TestProtoMappersReducePrecision(t *testing.T) {
	timeWithNanos := time.Date(2026, 11, 8, 12, 0, 0, 123456789, time.UTC)
	timeWithMillis := time.Date(2026, 11, 8, 12, 0, 0, 123000000, time.UTC)

	loadWithDoublePrecision := 412.738109123
	// The nearest float32 to loadWithDoublePrecision.
	loadWithFloatPrecision := 412.73809814453125

	t.Run("assigned state timestamps keep milliseconds", func(t *testing.T) {
		value, err := AssignedStateToProto(&store.AssignedState{
			ShardHandoverStats: map[string]store.ShardHandoverStats{
				"shard-1": {PreviousExecutorLastHeartbeatTime: timeWithNanos},
			},
			LastUpdated: timeWithNanos,
		})
		require.NoError(t, err)

		got, err := AssignedStateFromProto(value)
		require.NoError(t, err)
		assert.Equal(t, timeWithMillis, got.LastUpdated)
		assert.Equal(t, timeWithMillis, got.ShardHandoverStats["shard-1"].PreviousExecutorLastHeartbeatTime)
	})

	t.Run("reported load keeps float precision", func(t *testing.T) {
		value, err := ReportedShardsToProto(map[string]*types.ShardStatusReport{
			"shard-1": {Status: types.ShardStatusREADY, ShardLoad: loadWithDoublePrecision},
		})
		require.NoError(t, err)

		got, err := ReportedShardsFromProto(value)
		require.NoError(t, err)
		assert.Equal(t, loadWithFloatPrecision, got["shard-1"].ShardLoad)
	})

	t.Run("statistics keep milliseconds and float precision", func(t *testing.T) {
		value := ShardStatisticsToProto(map[string]store.ShardStatistics{
			"shard-1": {
				SmoothedLoad:   loadWithDoublePrecision,
				LastUpdateTime: timeWithNanos,
				LastMoveTime:   timeWithNanos,
			},
		})

		got := ShardStatisticsFromProto(value)["shard-1"]
		assert.Equal(t, loadWithFloatPrecision, got.SmoothedLoad)
		assert.Equal(t, timeWithMillis, got.LastUpdateTime)
		assert.Equal(t, timeWithMillis, got.LastMoveTime)
	})

	t.Run("zero timestamps are stored as zero", func(t *testing.T) {
		value := ShardStatisticsToProto(map[string]store.ShardStatistics{"shard-1": {}})
		storedStatistic := value.Shards["shard-1"]
		decodedStatistic := ShardStatisticsFromProto(value)["shard-1"]

		assert.Zero(t, storedStatistic.LastUpdateUnixMillis)
		assert.Zero(t, storedStatistic.LastMoveUnixMillis)
		assert.True(t, decodedStatistic.LastUpdateTime.IsZero())
		assert.True(t, decodedStatistic.LastMoveTime.IsZero())
	})
}

// roundTripBytes marshals value and unmarshals the bytes into decoded, the way a value
// is written to and read from etcd.
func roundTripBytes(t *testing.T, value, decoded proto.Message) {
	t.Helper()

	data, err := proto.Marshal(value)
	require.NoError(t, err)

	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)
}
