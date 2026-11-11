package common

import (
	"encoding/json"
	"fmt"

	"github.com/gogo/protobuf/proto"

	storev1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/store/v1"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdtypes"
)

// Encoding is the format executor values are serialized in before compression.
type Encoding int

const (
	EncodingJSON Encoding = iota
	EncodingProtobuf
)

// _protobufMarker is the first byte of a decompressed protobuf value. JSON values start
// with a printable character, so a value starting with this byte is never JSON.
const _protobufMarker byte = 0x01

// ExecutorValueEncoder encodes the executor status, reported shards, assigned state and
// shard statistics values. Decoding accepts both encodings regardless of the one the
// encoder writes, so values written by either encoding stay readable.
type ExecutorValueEncoder struct {
	recordWriter *RecordWriter
	encoding     Encoding
}

// NewExecutorValueEncoder constructs an ExecutorValueEncoder that writes the given encoding
// with the given compression.
func NewExecutorValueEncoder(compressionType string, encoding Encoding) (*ExecutorValueEncoder, error) {
	recordWriter, err := NewRecordWriter(compressionType)
	if err != nil {
		return nil, err
	}
	if err := validateEncoding(encoding); err != nil {
		return nil, err
	}
	return &ExecutorValueEncoder{recordWriter: recordWriter, encoding: encoding}, nil
}

// EncodeExecutorStatus encodes an executor status value.
func (e *ExecutorValueEncoder) EncodeExecutorStatus(status types.ExecutorStatus) ([]byte, error) {
	switch e.encoding {
	case EncodingJSON:
		return e.encodeJSON(status)
	case EncodingProtobuf:
		value, err := etcdtypes.ExecutorStatusToProto(status)
		if err != nil {
			return nil, err
		}
		return e.encodeProtobuf(value)
	default:
		return nil, unknownEncodingError(e.encoding)
	}
}

// EncodeReportedShards encodes a reported shards value.
func (e *ExecutorValueEncoder) EncodeReportedShards(reportedShards map[string]*types.ShardStatusReport) ([]byte, error) {
	switch e.encoding {
	case EncodingJSON:
		return e.encodeJSON(reportedShards)
	case EncodingProtobuf:
		value, err := etcdtypes.ReportedShardsToProto(reportedShards)
		if err != nil {
			return nil, err
		}
		return e.encodeProtobuf(value)
	default:
		return nil, unknownEncodingError(e.encoding)
	}
}

// EncodeAssignedState encodes an assigned state value.
func (e *ExecutorValueEncoder) EncodeAssignedState(assignedState *store.AssignedState) ([]byte, error) {
	switch e.encoding {
	case EncodingJSON:
		return e.encodeJSON(etcdtypes.FromAssignedState(assignedState))
	case EncodingProtobuf:
		value, err := etcdtypes.AssignedStateToProto(assignedState)
		if err != nil {
			return nil, err
		}
		return e.encodeProtobuf(value)
	default:
		return nil, unknownEncodingError(e.encoding)
	}
}

// EncodeShardStatistics encodes a shard statistics value.
func (e *ExecutorValueEncoder) EncodeShardStatistics(statistics map[string]store.ShardStatistics) ([]byte, error) {
	switch e.encoding {
	case EncodingJSON:
		return e.encodeJSON(etcdtypes.FromShardStatisticsMap(statistics))
	case EncodingProtobuf:
		value := etcdtypes.ShardStatisticsToProto(statistics)
		return e.encodeProtobuf(value)
	default:
		return nil, unknownEncodingError(e.encoding)
	}
}

func (e *ExecutorValueEncoder) encodeJSON(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}
	return e.compress(data)
}

func (e *ExecutorValueEncoder) encodeProtobuf(value proto.Message) ([]byte, error) {
	data, err := proto.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal protobuf: %w", err)
	}
	markedData := append([]byte{_protobufMarker}, data...)
	return e.compress(markedData)
}

func (e *ExecutorValueEncoder) compress(data []byte) ([]byte, error) {
	compressed, err := e.recordWriter.Write(data)
	if err != nil {
		return nil, fmt.Errorf("compress: %w", err)
	}
	return compressed, nil
}

// DecodeExecutorStatus decodes an executor status value in either encoding.
func DecodeExecutorStatus(value []byte) (types.ExecutorStatus, error) {
	data, isProtobuf, err := decompressValue(value)
	if err != nil {
		return types.ExecutorStatusINVALID, err
	}

	if isProtobuf {
		var stored storev1.ExecutorStatusValue
		if err := proto.Unmarshal(data, &stored); err != nil {
			return types.ExecutorStatusINVALID, fmt.Errorf("unmarshal protobuf: %w", err)
		}
		return etcdtypes.ExecutorStatusFromProto(&stored)
	}

	var status types.ExecutorStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return types.ExecutorStatusINVALID, fmt.Errorf("unmarshal json: %w", err)
	}
	return status, nil
}

// DecodeReportedShards decodes a reported shards value in either encoding.
func DecodeReportedShards(value []byte) (map[string]*types.ShardStatusReport, error) {
	data, isProtobuf, err := decompressValue(value)
	if err != nil {
		return nil, err
	}

	if isProtobuf {
		var stored storev1.ReportedShardsValue
		if err := proto.Unmarshal(data, &stored); err != nil {
			return nil, fmt.Errorf("unmarshal protobuf: %w", err)
		}
		return etcdtypes.ReportedShardsFromProto(&stored)
	}

	var reportedShards map[string]*types.ShardStatusReport
	if err := json.Unmarshal(data, &reportedShards); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}
	return reportedShards, nil
}

// DecodeAssignedState decodes an assigned state value in either encoding. The mod
// revision is not part of the value, so the caller sets it from the etcd key.
func DecodeAssignedState(value []byte) (*store.AssignedState, error) {
	data, isProtobuf, err := decompressValue(value)
	if err != nil {
		return nil, err
	}

	if isProtobuf {
		var stored storev1.AssignedStateValue
		if err := proto.Unmarshal(data, &stored); err != nil {
			return nil, fmt.Errorf("unmarshal protobuf: %w", err)
		}
		return etcdtypes.AssignedStateFromProto(&stored)
	}

	var stored etcdtypes.AssignedState
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}
	return stored.ToAssignedState(), nil
}

// DecodeShardStatistics decodes a shard statistics value in either encoding.
func DecodeShardStatistics(value []byte) (map[string]store.ShardStatistics, error) {
	data, isProtobuf, err := decompressValue(value)
	if err != nil {
		return nil, err
	}

	if isProtobuf {
		var stored storev1.ShardStatisticsValue
		if err := proto.Unmarshal(data, &stored); err != nil {
			return nil, fmt.Errorf("unmarshal protobuf: %w", err)
		}
		return etcdtypes.ShardStatisticsFromProto(&stored), nil
	}

	var stored map[string]etcdtypes.ShardStatistics
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}
	return etcdtypes.ToShardStatisticsMap(stored), nil
}

// decompressValue decompresses a value and reports whether it is protobuf. For a
// protobuf value, the returned data no longer includes the marker.
func decompressValue(value []byte) ([]byte, bool, error) {
	data, err := Decompress(value)
	if err != nil {
		return nil, false, fmt.Errorf("decompress: %w", err)
	}
	if len(data) > 0 && data[0] == _protobufMarker {
		return data[1:], true, nil
	}
	return data, false, nil
}

func validateEncoding(encoding Encoding) error {
	switch encoding {
	case EncodingJSON, EncodingProtobuf:
		return nil
	default:
		return unknownEncodingError(encoding)
	}
}

func unknownEncodingError(encoding Encoding) error {
	return fmt.Errorf("unknown executor value encoding %d", encoding)
}
