package common

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.etcd.io/etcd/api/v3/mvccpb"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdkeys"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdtypes"
)

// ParseExecutorKVs parses a list of etcd key-value pairs into a map of ParsedExecutorData,
// grouped by executor ID.
func ParseExecutorKVs(etcdPrefix, namespace string, kvs []*mvccpb.KeyValue) (map[string]*etcdtypes.ParsedExecutorData, error) {
	data := make(map[string]*etcdtypes.ParsedExecutorData)

	for _, kv := range kvs {
		executorID, keyType, err := etcdkeys.ParseExecutorKey(etcdPrefix, namespace, string(kv.Key))
		if err != nil {
			return nil, fmt.Errorf("parse executor key %s: %w", string(kv.Key), err)
		}

		execData, ok := data[executorID]
		if !ok {
			execData = &etcdtypes.ParsedExecutorData{
				ReportedShards: make(map[string]*types.ShardStatusReport),
				Metadata:       make(map[string]string),
				Statistics:     make(map[string]etcdtypes.ShardStatistics),
			}
			data[executorID] = execData
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
		case etcdkeys.ExecutorHostMetadataKey:
			var hostMetadata types.HostMetadata
			if err := json.Unmarshal(kv.Value, &hostMetadata); err != nil {
				return nil, fmt.Errorf("parse host metadata for %s: %w", executorID, err)
			}
			execData.HostMetadata = &hostMetadata
		}
	}

	return data, nil
}
