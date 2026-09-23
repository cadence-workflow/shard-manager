package common

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.etcd.io/etcd/api/v3/mvccpb"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdkeys"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdtypes"
)

// ParseExecutorKVs parses a list of etcd key-value pairs into a map of ParsedExecutorData,
// grouped by executor ID.
// Unknown key types are ignored and do not create an executor.
func ParseExecutorKVs(etcdPrefix, namespace string, kvs []*mvccpb.KeyValue) (map[string]*etcdtypes.ParsedExecutorData, error) {
	data := make(map[string]*etcdtypes.ParsedExecutorData)

	for _, kv := range kvs {
		executorID, keyType, err := etcdkeys.ParseExecutorKey(etcdPrefix, namespace, string(kv.Key))
		if err != nil {
			return nil, fmt.Errorf("parse executor key %s: %w", string(kv.Key), err)
		}

		execData, ok := data[executorID]
		if !ok {
			execData = newParsedExecutorData()
		}

		switch keyType {
		case etcdkeys.ExecutorHeartbeatKey:
			heartbeatTime, err := etcdtypes.ParseTime(string(kv.Value))
			if err != nil {
				return nil, fmt.Errorf("parse heartbeat time for %s: %w", executorID, err)
			}
			execData.LastHeartbeat = etcdtypes.Time(heartbeatTime)
		case etcdkeys.ExecutorStatusKey:
			if err := DecompressAndUnmarshal(kv.Value, &execData.Status); err != nil {
				return nil, fmt.Errorf("parse executor status for %s: %w", executorID, err)
			}
		case etcdkeys.ExecutorReportedShardsKey:
			if err := DecompressAndUnmarshal(kv.Value, &execData.ReportedShards); err != nil {
				return nil, fmt.Errorf("parse reported shards for %s: %w", executorID, err)
			}
		case etcdkeys.ExecutorAssignedStateKey:
			var assignedState etcdtypes.AssignedState
			if err := DecompressAndUnmarshal(kv.Value, &assignedState); err != nil {
				return nil, fmt.Errorf("parse assigned state for %s: %w", executorID, err)
			}
			assignedState.ModRevision = kv.ModRevision
			execData.AssignedState = &assignedState
		case etcdkeys.ExecutorMetadataKey:
			metadataKey := strings.TrimPrefix(string(kv.Key), etcdkeys.BuildMetadataKey(etcdPrefix, namespace, executorID, ""))
			execData.Metadata[metadataKey] = string(kv.Value)
		case etcdkeys.ExecutorShardStatisticsKey:
			if err := DecompressAndUnmarshal(kv.Value, &execData.Statistics); err != nil {
				return nil, fmt.Errorf("parse shard statistics for %s: %w", executorID, err)
			}
		default:
			// Skip keys from newer binaries
			continue
		}

		data[executorID] = execData
	}

	return data, nil
}

func newParsedExecutorData() *etcdtypes.ParsedExecutorData {
	return &etcdtypes.ParsedExecutorData{
		ReportedShards: make(map[string]*types.ShardStatusReport),
		Metadata:       make(map[string]string),
		Statistics:     make(map[string]etcdtypes.ShardStatistics),
	}
}

// ParseExecutorAssignmentKVs decodes only the assignment and metadata keys, leaving the
// compressed statistics and reported shards untouched. Any known executor key makes the
// executor visible, so one with no assignment still gets a metadata entry.
func ParseExecutorAssignmentKVs(etcdPrefix, namespace string, kvs []*mvccpb.KeyValue) (map[string]map[string]string, map[string]store.AssignedState, error) {
	metadata := make(map[string]map[string]string)
	assignments := make(map[string]store.AssignedState)

	for _, kv := range kvs {
		executorID, keyType, err := etcdkeys.ParseExecutorKey(etcdPrefix, namespace, string(kv.Key))
		if err != nil {
			return nil, nil, fmt.Errorf("parse executor key %s: %w", string(kv.Key), err)
		}

		executorMetadata, ok := metadata[executorID]
		if !ok {
			executorMetadata = make(map[string]string)
		}

		switch keyType {
		case etcdkeys.ExecutorAssignedStateKey:
			var assignedState etcdtypes.AssignedState
			if err := DecompressAndUnmarshal(kv.Value, &assignedState); err != nil {
				return nil, nil, fmt.Errorf("parse assigned state for %s: %w", executorID, err)
			}
			assignedState.ModRevision = kv.ModRevision
			assignments[executorID] = *assignedState.ToAssignedState()
		case etcdkeys.ExecutorMetadataKey:
			metadataKey := strings.TrimPrefix(string(kv.Key), etcdkeys.BuildMetadataKey(etcdPrefix, namespace, executorID, ""))
			executorMetadata[metadataKey] = string(kv.Value)
		case etcdkeys.ExecutorHeartbeatKey, etcdkeys.ExecutorStatusKey, etcdkeys.ExecutorReportedShardsKey, etcdkeys.ExecutorShardStatisticsKey:
			// Not read here, but the key still proves the executor exists.
		default:
			// Skip keys from newer binaries
			continue
		}

		metadata[executorID] = executorMetadata
	}

	return metadata, assignments, nil
}
