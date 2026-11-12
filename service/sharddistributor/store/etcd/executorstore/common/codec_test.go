package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

func TestExecutorValueEncoderRoundTrip(t *testing.T) {
	lastUpdated := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC).Add(123 * time.Millisecond)
	// float32 represents this load exactly, so both encodings return it unchanged.
	shardLoad := 412.5

	status := types.ExecutorStatusDRAINING
	reportedShards := map[string]*types.ShardStatusReport{
		"shard-1": {Status: types.ShardStatusREADY, ShardLoad: shardLoad},
	}
	assignedState := &store.AssignedState{
		AssignedShards: map[string]*types.ShardAssignment{
			"shard-1": {Status: types.AssignmentStatusREADY},
		},
		ShardHandoverStats: map[string]store.ShardHandoverStats{
			"shard-1": {PreviousExecutorLastHeartbeatTime: lastUpdated, HandoverType: types.HandoverTypeEMERGENCY},
		},
		LastUpdated: lastUpdated,
	}
	statistics := map[string]store.ShardStatistics{
		"shard-1": {SmoothedLoad: shardLoad, LastUpdateTime: lastUpdated, LastMoveTime: lastUpdated},
	}

	tests := map[string]struct {
		compression string
		encoding    Encoding
	}{
		"json with snappy":      {compression: CompressionSnappy, encoding: EncodingJSON},
		"json uncompressed":     {compression: "none", encoding: EncodingJSON},
		"protobuf with snappy":  {compression: CompressionSnappy, encoding: EncodingProtobuf},
		"protobuf uncompressed": {compression: "none", encoding: EncodingProtobuf},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			encoder, err := NewExecutorValueEncoder(tt.compression, tt.encoding)
			require.NoError(t, err)

			encodedStatus, err := encoder.EncodeExecutorStatus(status)
			require.NoError(t, err)
			gotStatus, err := DecodeExecutorStatus(encodedStatus)
			require.NoError(t, err)
			assert.Equal(t, status, gotStatus)

			encodedReportedShards, err := encoder.EncodeReportedShards(reportedShards)
			require.NoError(t, err)
			gotReportedShards, err := DecodeReportedShards(encodedReportedShards)
			require.NoError(t, err)
			assert.Equal(t, reportedShards, gotReportedShards)

			encodedAssignedState, err := encoder.EncodeAssignedState(assignedState)
			require.NoError(t, err)
			gotAssignedState, err := DecodeAssignedState(encodedAssignedState)
			require.NoError(t, err)
			assert.Equal(t, assignedState, gotAssignedState)

			encodedStatistics, err := encoder.EncodeShardStatistics(statistics)
			require.NoError(t, err)
			gotStatistics, err := DecodeShardStatistics(encodedStatistics)
			require.NoError(t, err)
			assert.Equal(t, statistics, gotStatistics)
		})
	}
}

// Decoding treats a value starting with the protobuf marker as protobuf, so a JSON value
// must never start with it.
func TestJSONValuesNeverStartWithProtobufMarker(t *testing.T) {
	encoder, err := NewExecutorValueEncoder("none", EncodingJSON)
	require.NoError(t, err)

	encodedStatus, err := encoder.EncodeExecutorStatus(types.ExecutorStatusACTIVE)
	require.NoError(t, err)
	encodedReportedShards, err := encoder.EncodeReportedShards(map[string]*types.ShardStatusReport{})
	require.NoError(t, err)
	encodedAssignedState, err := encoder.EncodeAssignedState(&store.AssignedState{})
	require.NoError(t, err)
	encodedStatistics, err := encoder.EncodeShardStatistics(map[string]store.ShardStatistics{})
	require.NoError(t, err)

	for _, encoded := range [][]byte{encodedStatus, encodedReportedShards, encodedAssignedState, encodedStatistics} {
		require.NotEmpty(t, encoded)
		assert.NotEqual(t, _protobufMarker, encoded[0], "value %q", encoded)
	}
}

func TestDecodeRejectsCorruptValues(t *testing.T) {
	tests := map[string]struct {
		value   []byte
		wantErr string
	}{
		"invalid json": {
			value:   []byte(`{invalid json}`),
			wantErr: "unmarshal json",
		},
		"invalid protobuf": {
			value:   []byte{_protobufMarker, 0xff, 0xff},
			wantErr: "unmarshal protobuf",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeAssignedState(tt.value)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestNewExecutorValueEncoderRejectsUnknownEncoding(t *testing.T) {
	unknownEncoding := Encoding(99)

	_, err := NewExecutorValueEncoder(CompressionSnappy, unknownEncoding)
	require.ErrorContains(t, err, "unknown executor value encoding 99")
}
