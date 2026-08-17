package executorstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/api/v3/etcdserverpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/fx/fxtest"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v2"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/dynamicconfig/dynamicproperties"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/statistics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdclient"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdkeys"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdtypes"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/executorstore/common"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/leaderstore"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/testhelper"
)

// TestRecordHeartbeat verifies that an executor's heartbeat is correctly stored.
func TestRecordHeartbeat(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now().UTC()

	executorID := "executor-TestRecordHeartbeat"
	req := store.HeartbeatState{
		LastHeartbeat: now,
		Status:        types.ExecutorStatusACTIVE,
		ReportedShards: map[string]*types.ShardStatusReport{
			"shard-TestRecordHeartbeat": {Status: types.ShardStatusREADY},
		},
		Metadata: map[string]string{
			"key-1": "value-1",
			"key-2": "value-2",
		},
	}

	err := executorStore.RecordHeartbeat(ctx, tc.Namespace, executorID, req)
	require.NoError(t, err)

	// Verify directly in etcd
	heartbeatKey := etcdkeys.BuildExecutorKey(tc.EtcdPrefix, tc.Namespace, executorID, etcdkeys.ExecutorHeartbeatKey)
	stateKey := etcdkeys.BuildExecutorKey(tc.EtcdPrefix, tc.Namespace, executorID, etcdkeys.ExecutorStatusKey)
	reportedShardsKey := etcdkeys.BuildExecutorKey(tc.EtcdPrefix, tc.Namespace, executorID, etcdkeys.ExecutorReportedShardsKey)
	metadataKey1 := etcdkeys.BuildMetadataKey(tc.EtcdPrefix, tc.Namespace, executorID, "key-1")
	metadataKey2 := etcdkeys.BuildMetadataKey(tc.EtcdPrefix, tc.Namespace, executorID, "key-2")

	resp, err := tc.Client.Get(ctx, heartbeatKey)
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.Count, "Heartbeat key should exist")
	assert.Equal(t, etcdtypes.FormatTime(now), string(resp.Kvs[0].Value))

	resp, err = tc.Client.Get(ctx, stateKey)
	require.NoError(t, err)
	require.Equal(t, int64(1), resp.Count, "State key should exist")
	decompressedState, err := common.Decompress(resp.Kvs[0].Value)
	require.NoError(t, err)
	assert.Equal(t, stringStatus(types.ExecutorStatusACTIVE), string(decompressedState))

	resp, err = tc.Client.Get(ctx, reportedShardsKey)
	require.NoError(t, err)
	require.Equal(t, int64(1), resp.Count, "Reported shards key should exist")

	decompressedReportedShards, err := common.Decompress(resp.Kvs[0].Value)
	require.NoError(t, err)
	var reportedShards map[string]*types.ShardStatusReport
	err = json.Unmarshal(decompressedReportedShards, &reportedShards)
	require.NoError(t, err)
	require.Len(t, reportedShards, 1)
	assert.Equal(t, types.ShardStatusREADY, reportedShards["shard-TestRecordHeartbeat"].Status)

	resp, err = tc.Client.Get(ctx, metadataKey1)
	require.NoError(t, err)
	require.Equal(t, int64(1), resp.Count, "Metadata key 1 should exist")
	assert.Equal(t, "value-1", string(resp.Kvs[0].Value))

	resp, err = tc.Client.Get(ctx, metadataKey2)
	require.NoError(t, err)
	require.Equal(t, int64(1), resp.Count, "Metadata key 2 should exist")
	assert.Equal(t, "value-2", string(resp.Kvs[0].Value))
}

func TestRecordHeartbeat_NoCompression(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)

	var etcdCfg struct {
		Endpoints   []string      `yaml:"endpoints"`
		DialTimeout time.Duration `yaml:"dialTimeout"`
		Prefix      string        `yaml:"prefix"`
		Compression string        `yaml:"compression"`
	}
	require.NoError(t, tc.SDConfig.Store.StorageParams.Decode(&etcdCfg))
	etcdCfg.Compression = "none"

	encodedCfg, err := yaml.Marshal(etcdCfg)
	require.NoError(t, err)

	var yamlNode *config.YamlNode
	require.NoError(t, yaml.Unmarshal(encodedCfg, &yamlNode))
	tc.SDConfig.Store.StorageParams = yamlNode
	tc.SDConfig.LeaderStore.StorageParams = yamlNode
	tc.Compression = "none"

	executorStore := createStore(t, tc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	executorID := "executor-no-compression"
	req := store.HeartbeatState{
		LastHeartbeat: time.Now().UTC(),
		Status:        types.ExecutorStatusACTIVE,
		ReportedShards: map[string]*types.ShardStatusReport{
			"shard-no-compression": {Status: types.ShardStatusREADY},
		},
	}

	require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, executorID, req))

	stateKey := etcdkeys.BuildExecutorKey(tc.EtcdPrefix, tc.Namespace, executorID, etcdkeys.ExecutorStatusKey)
	require.NoError(t, err)
	reportedShardsKey := etcdkeys.BuildExecutorKey(tc.EtcdPrefix, tc.Namespace, executorID, etcdkeys.ExecutorReportedShardsKey)
	require.NoError(t, err)

	stateResp, err := tc.Client.Get(ctx, stateKey)
	require.NoError(t, err)
	require.Equal(t, int64(1), stateResp.Count)
	statusJSON, err := json.Marshal(req.Status)
	require.NoError(t, err)
	assert.Equal(t, string(statusJSON), string(stateResp.Kvs[0].Value))

	reportedResp, err := tc.Client.Get(ctx, reportedShardsKey)
	require.NoError(t, err)
	require.Equal(t, int64(1), reportedResp.Count)
	reportedJSON, err := json.Marshal(req.ReportedShards)
	require.NoError(t, err)
	assert.Equal(t, string(reportedJSON), string(reportedResp.Kvs[0].Value))
}

func TestRecordHeartbeatUpdatesShardStatistics(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	setLoadBalancingMode(executorStore, config.LoadBalancingModeGREEDY)
	setLoadSmoothingTimeConstant(executorStore, 90*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	executorID := "executor-shard-stats"
	shardID := "shard-with-load"

	require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, executorID, store.HeartbeatState{Status: types.ExecutorStatusACTIVE}))
	require.NoError(t, executorStore.AssignShard(ctx, tc.Namespace, shardID, executorID))

	impl := executorStore.(*executorStoreImpl)
	assert.Eventually(t, func() bool {
		owner, err := impl.shardCache.GetShardOwner(ctx, tc.Namespace, shardID)
		return err == nil && owner.ExecutorID == executorID
	}, 5*time.Second, 50*time.Millisecond)

	stateBeforeHeartbeat, err := executorStore.GetState(ctx, tc.Namespace)
	require.NoError(t, err)
	beforeStats, ok := stateBeforeHeartbeat.ShardStats[shardID]
	require.True(t, ok)
	assert.True(t, beforeStats.LastUpdateTime.IsZero(), "assigned shard should be unmeasured until first heartbeat")

	impl.timeSource.(clock.MockedTimeSource).Advance(5 * time.Second)

	req := store.HeartbeatState{
		LastHeartbeat: impl.timeSource.Now().UTC(),
		Status:        types.ExecutorStatusACTIVE,
		ReportedShards: map[string]*types.ShardStatusReport{
			shardID: {
				Status:    types.ShardStatusREADY,
				ShardLoad: 45.6,
			},
		},
	}

	require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, executorID, req))

	nsState, err := executorStore.GetState(ctx, tc.Namespace)
	require.NoError(t, err)

	updated, ok := nsState.ShardStats[shardID]
	require.True(t, ok)
	assert.True(t, updated.LastUpdateTime.After(beforeStats.LastUpdateTime))
	assert.InDelta(t, req.ReportedShards[shardID].ShardLoad, updated.SmoothedLoad, 1e-9)
	assert.Equal(t, beforeStats.LastMoveTime, updated.LastMoveTime)
}

func TestRecordHeartbeatSkipsShardStatisticsWithNilReport(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	setLoadBalancingMode(executorStore, config.LoadBalancingModeGREEDY)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	executorID := "executor-missing-load"
	validShardID := "shard-with-valid-load"
	skippedShardID := "shard-missing-load"

	require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, executorID, store.HeartbeatState{Status: types.ExecutorStatusACTIVE}))
	require.NoError(t, executorStore.AssignShard(ctx, tc.Namespace, validShardID, executorID))

	impl := executorStore.(*executorStoreImpl)
	assert.Eventually(t, func() bool {
		owner, err := impl.shardCache.GetShardOwner(ctx, tc.Namespace, validShardID)
		return err == nil && owner.ExecutorID == executorID
	}, 5*time.Second, 50*time.Millisecond)

	stateBeforeHeartbeat, err := executorStore.GetState(ctx, tc.Namespace)
	require.NoError(t, err)
	beforeStats, ok := stateBeforeHeartbeat.ShardStats[validShardID]
	require.True(t, ok)

	impl.timeSource.(clock.MockedTimeSource).Advance(5 * time.Second)

	req := store.HeartbeatState{
		LastHeartbeat: impl.timeSource.Now().UTC(),
		Status:        types.ExecutorStatusACTIVE,
		ReportedShards: map[string]*types.ShardStatusReport{
			validShardID: {
				Status:    types.ShardStatusREADY,
				ShardLoad: 3.21,
			},
			skippedShardID: nil,
		},
	}

	require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, executorID, req))

	nsState, err := executorStore.GetState(ctx, tc.Namespace)
	require.NoError(t, err)

	validStats, ok := nsState.ShardStats[validShardID]
	require.True(t, ok)
	expectedLoad, err := statistics.CalculateSmoothedLoad(beforeStats.SmoothedLoad, req.ReportedShards[validShardID].ShardLoad, beforeStats.LastUpdateTime, validStats.LastUpdateTime, statistics.DefaultLoadSmoothingTimeConstant)
	require.NoError(t, err)
	assert.InDelta(t, expectedLoad, validStats.SmoothedLoad, 1e-9)
	assert.False(t, validStats.LastUpdateTime.IsZero())
	assert.Equal(t, beforeStats.LastMoveTime, validStats.LastMoveTime)

	assert.NotContains(t, nsState.ShardStats, skippedShardID)
}

func TestGetHeartbeat(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now().UTC()

	executorID := "executor-get"
	req := store.HeartbeatState{
		Status:        types.ExecutorStatusDRAINING,
		LastHeartbeat: now,
	}

	// 1. Record a heartbeat
	err := executorStore.RecordHeartbeat(ctx, tc.Namespace, executorID, req)
	require.NoError(t, err)

	// Assign shards to one executor
	assignState := map[string]store.AssignedState{
		executorID: {
			AssignedShards: map[string]*types.ShardAssignment{
				"shard-1": {Status: types.AssignmentStatusREADY},
			},
		},
	}
	require.NoError(t, executorStore.AssignShards(ctx, tc.Namespace, store.AssignShardsRequest{
		NewState: &store.NamespaceState{
			ShardAssignments: assignState,
		},
	}, store.NopGuard()))

	// 2. Get the heartbeat back
	hb, assignedFromDB, err := executorStore.GetHeartbeat(ctx, tc.Namespace, executorID)
	require.NoError(t, err)
	require.NotNil(t, hb)

	// 3. Verify the state
	assert.Equal(t, types.ExecutorStatusDRAINING, hb.Status)
	assert.Equal(t, now, hb.LastHeartbeat)
	require.NotNil(t, assignedFromDB.AssignedShards)
	assert.Equal(t, assignState[executorID].AssignedShards, assignedFromDB.AssignedShards)

	// 4. Test getting a non-existent executor
	_, _, err = executorStore.GetHeartbeat(ctx, tc.Namespace, "executor-non-existent")
	require.Error(t, err)
	assert.ErrorIs(t, err, store.ErrExecutorNotFound)
}

// TestGetState verifies that the store can accurately retrieve the state of all executors.
func TestGetState(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	executorID1 := "exec-TestGetState-1"
	executorID2 := "exec-TestGetState-2"
	shardID1 := "shard-1"
	shardID2 := "shard-2"

	// Setup: Record heartbeats and assign shards.
	require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, executorID1, store.HeartbeatState{Status: types.ExecutorStatusACTIVE}))
	require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, executorID2, store.HeartbeatState{Status: types.ExecutorStatusDRAINING}))
	require.NoError(t, executorStore.AssignShards(ctx, tc.Namespace, store.AssignShardsRequest{
		NewState: &store.NamespaceState{
			ShardAssignments: map[string]store.AssignedState{
				executorID1: {AssignedShards: map[string]*types.ShardAssignment{shardID1: {}}},
				executorID2: {AssignedShards: map[string]*types.ShardAssignment{shardID2: {}}},
			},
		},
	}, store.NopGuard()))

	// Action: Get the state.
	namespaceState, err := executorStore.GetState(ctx, tc.Namespace)
	require.NoError(t, err)

	// Verification:
	// Check Executors
	require.Len(t, namespaceState.Executors, 2, "Should retrieve two heartbeat states")
	assert.Equal(t, types.ExecutorStatusACTIVE, namespaceState.Executors[executorID1].Status)
	assert.Equal(t, types.ExecutorStatusDRAINING, namespaceState.Executors[executorID2].Status)

	// Check ShardAssignments (from executor records)
	require.Len(t, namespaceState.ShardAssignments, 2, "Should retrieve two assignment states")
	assert.Contains(t, namespaceState.ShardAssignments[executorID1].AssignedShards, shardID1)
	assert.Contains(t, namespaceState.ShardAssignments[executorID2].AssignedShards, shardID2)
}

// TestAssignShards_WithRevisions tests the optimistic locking logic of AssignShards.
func TestAssignShards_WithRevisions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	executorID1 := "exec-rev-1"
	executorID2 := "exec-rev-2"

	t.Run("Success", func(t *testing.T) {
		tc := testhelper.SetupStoreTestCluster(t)
		executorStore := createStore(t, tc)
		recordHeartbeats(ctx, t, executorStore, tc.Namespace, executorID1, executorID2)

		// Define a new state: assign shard1 to exec1
		newState := &store.NamespaceState{
			ShardAssignments: map[string]store.AssignedState{
				executorID1: {AssignedShards: map[string]*types.ShardAssignment{"shard-1": {}}},
			},
		}

		// Assign - should succeed
		err := executorStore.AssignShards(ctx, tc.Namespace, store.AssignShardsRequest{NewState: newState}, store.NopGuard())
		require.NoError(t, err)

		// Verify the assignment
		state, err := executorStore.GetState(ctx, tc.Namespace)
		require.NoError(t, err)
		assert.Contains(t, state.ShardAssignments[executorID1].AssignedShards, "shard-1")
	})

	t.Run("ConflictOnNewShard", func(t *testing.T) {
		tc := testhelper.SetupStoreTestCluster(t)
		executorStore := createStore(t, tc)
		recordHeartbeats(ctx, t, executorStore, tc.Namespace, executorID1, executorID2)

		// Process A defines its desired state: assign shard-new to exec1
		processAState := &store.NamespaceState{
			ShardAssignments: map[string]store.AssignedState{
				executorID1: {AssignedShards: map[string]*types.ShardAssignment{"shard-new": {}}},
				executorID2: {},
			},
		}

		// Process B defines its desired state: assign shard-new to exec2
		processBState := &store.NamespaceState{
			ShardAssignments: map[string]store.AssignedState{
				executorID1: {},
				executorID2: {AssignedShards: map[string]*types.ShardAssignment{"shard-new": {}}},
			},
		}

		// Process A succeeds
		err := executorStore.AssignShards(ctx, tc.Namespace, store.AssignShardsRequest{NewState: processAState}, store.NopGuard())
		require.NoError(t, err)

		// Process B tries to commit, but its revision check for shard-new (rev=0) will fail.
		err = executorStore.AssignShards(ctx, tc.Namespace, store.AssignShardsRequest{NewState: processBState}, store.NopGuard())
		require.Error(t, err)
		assert.ErrorIs(t, err, store.ErrVersionConflict)
	})

	t.Run("ConflictOnExistingShard", func(t *testing.T) {
		tc := testhelper.SetupStoreTestCluster(t)
		executorStore := createStore(t, tc)
		recordHeartbeats(ctx, t, executorStore, tc.Namespace, executorID1, executorID2)

		shardID := "shard-to-move"
		// 1. Setup: Assign the shard to executor1
		setupState, err := executorStore.GetState(ctx, tc.Namespace)
		require.NoError(t, err)
		setupState.ShardAssignments = map[string]store.AssignedState{
			executorID1: {AssignedShards: map[string]*types.ShardAssignment{shardID: {}}},
		}
		require.NoError(t, executorStore.AssignShards(ctx, tc.Namespace, store.AssignShardsRequest{NewState: setupState}, store.NopGuard()))

		// 2. Process A reads the state, intending to move the shard to executor2
		stateForProcA, err := executorStore.GetState(ctx, tc.Namespace)
		require.NoError(t, err)
		stateForProcA.ShardAssignments = map[string]store.AssignedState{
			executorID1: {ModRevision: stateForProcA.ShardAssignments[executorID1].ModRevision},
			executorID2: {AssignedShards: map[string]*types.ShardAssignment{shardID: {}}, ModRevision: 0},
		}

		// 3. In the meantime, another process makes a different change (e.g., re-assigns to same executor, which changes revision)
		intermediateState, err := executorStore.GetState(ctx, tc.Namespace)
		require.NoError(t, err)
		intermediateState.ShardAssignments = map[string]store.AssignedState{
			executorID1: {
				AssignedShards: map[string]*types.ShardAssignment{shardID: {}},
				ModRevision:    intermediateState.ShardAssignments[executorID1].ModRevision,
			},
		}
		require.NoError(t, executorStore.AssignShards(ctx, tc.Namespace, store.AssignShardsRequest{NewState: intermediateState}, store.NopGuard()))

		// 4. Process A tries to commit its change. It will fail because its stored revision for the shard is now stale.
		err = executorStore.AssignShards(ctx, tc.Namespace, store.AssignShardsRequest{NewState: stateForProcA}, store.NopGuard())
		require.Error(t, err)
		assert.ErrorIs(t, err, store.ErrVersionConflict)
	})

	t.Run("NoChanges", func(t *testing.T) {
		tc := testhelper.SetupStoreTestCluster(t)
		executorStore := createStore(t, tc)
		recordHeartbeats(ctx, t, executorStore, tc.Namespace, executorID1, executorID2)

		// Get the current state
		state, err := executorStore.GetState(ctx, tc.Namespace)
		require.NoError(t, err)

		// Call AssignShards with the same assignments
		err = executorStore.AssignShards(ctx, tc.Namespace, store.AssignShardsRequest{NewState: state}, store.NopGuard())
		require.NoError(t, err, "Assigning with no changes should succeed")
	})
}

// TestGuardedOperations verifies that AssignShards and DeleteExecutors respect the leader guard.
func TestGuardedOperations(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	namespace := "test-guarded-ns"
	executorID := "exec-to-delete"

	// 1. Create two potential leaders
	leaderCfg, err := etcdclient.NewLeaderStoreConfig(tc.SDConfig)
	require.NoError(t, err)
	elector, err := leaderstore.NewLeaderStore(leaderstore.StoreParams{Client: tc.Client, Cfg: leaderCfg})
	require.NoError(t, err)
	election1, err := elector.CreateElection(ctx, namespace)
	defer election1.Cleanup(ctx)
	defer func() { _ = election1.Cleanup(ctx) }()
	election2, err := elector.CreateElection(ctx, namespace)
	defer election2.Cleanup(ctx)
	defer func() { _ = election2.Cleanup(ctx) }()

	// 2. First node becomes leader
	require.NoError(t, election1.Campaign(ctx, "host-1"))
	validGuard := election1.Guard()

	// 3. Use the valid guard to assign shards - should succeed
	assignState := map[string]store.AssignedState{"exec-1": {}}
	err = executorStore.AssignShards(ctx, tc.Namespace, store.AssignShardsRequest{NewState: &store.NamespaceState{ShardAssignments: assignState}}, validGuard)
	require.NoError(t, err, "Assigning shards with a valid leader guard should succeed")

	// 4. First node resigns, second node becomes leader
	require.NoError(t, election1.Resign(ctx))
	require.NoError(t, election2.Campaign(ctx, "host-2"))

	// 5. Use the now-invalid guard from the first leader - should fail
	err = executorStore.AssignShards(ctx, tc.Namespace, store.AssignShardsRequest{NewState: &store.NamespaceState{ShardAssignments: assignState}}, validGuard)
	require.Error(t, err, "Assigning shards with a stale leader guard should fail")

	// 6. Use the NopGuard to delete an executor - should succeed
	require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, executorID, store.HeartbeatState{Status: types.ExecutorStatusACTIVE}))
	err = executorStore.DeleteExecutors(ctx, tc.Namespace, []string{executorID}, store.NopGuard())
	require.NoError(t, err, "Deleting an executor without a guard should succeed")

	// Verify deletion
	newState, err := executorStore.GetState(ctx, namespace)
	require.NoError(t, err)
	_, ok := newState.ShardAssignments[executorID]
	require.False(t, ok, "Executor should have been deleted")
}

// TestSubscribe verifies that the subscription channel receives notifications for significant changes.
func TestSubscribeToExecutorStatusChanges(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	executorID := "exec-sub"

	// Start subscription
	sub, err := executorStore.SubscribeToExecutorStatusChanges(ctx, tc.Namespace)
	require.NoError(t, err)

	// Test case #1: Update heartbeat without changing status or reported shards - should NOT trigger notification
	{
		// Manually put a heartbeat update, which is an insignificant change
		heartbeatKey := etcdkeys.BuildExecutorKey(tc.EtcdPrefix, tc.Namespace, executorID, "heartbeat")
		_, err = tc.Client.Put(ctx, heartbeatKey, "timestamp")
		require.NoError(t, err)

		select {
		case <-sub:
			t.Fatal("Should not receive notification for a heartbeat-only update")
		case <-time.After(100 * time.Millisecond):
			// Expected behavior
		}
	}

	// Test case #2: Update reported shards without changing status - should NOT trigger notification
	{
		// Manually put a reported shards update, which is an insignificant change
		reportedShardsKey := etcdkeys.BuildExecutorKey(tc.EtcdPrefix, tc.Namespace, executorID, "reported_shards")
		writer, err := common.NewRecordWriter(tc.Compression)
		require.NoError(t, err)
		compressedShards, err := writer.Write([]byte(`{"shard-1":{"status":"running"}}`))
		require.NoError(t, err)
		_, err = tc.Client.Put(ctx, reportedShardsKey, string(compressedShards))
		require.NoError(t, err)

		select {
		case <-sub:
			t.Fatal("Should not receive notification for a reported-shards-only update")
		case <-time.After(100 * time.Millisecond):
			// Expected behavior
		}
	}

	// Test case #3: Update status without prevKV - should trigger notification
	{
		statusKey := etcdkeys.BuildExecutorKey(tc.EtcdPrefix, tc.Namespace, executorID, "status")
		_, err = tc.Client.Put(ctx, statusKey, stringStatus(types.ExecutorStatusDRAINING))
		require.NoError(t, err)

		select {
		case rev, ok := <-sub:
			require.True(t, ok, "Channel should be open")
			assert.Greater(t, rev, int64(0), "Should receive a valid revision for status change")
		case <-time.After(1 * time.Second):
			t.Fatal("Should have received a notification for a status change")
		}
	}

	// Test case #4: Update status with prevKV but the same value - should NOT trigger notification
	{
		statusKey := etcdkeys.BuildExecutorKey(tc.EtcdPrefix, tc.Namespace, executorID, "status")
		_, err = tc.Client.Put(ctx, statusKey, stringStatus(types.ExecutorStatusDRAINING))
		require.NoError(t, err)

		select {
		case <-sub:
			t.Fatal("Should not receive notification")
		case <-time.After(100 * time.Millisecond):
			// Expected behavior
		}
	}

	// Test case #5: Update status with prevKV - should trigger notification
	{
		statusKey := etcdkeys.BuildExecutorKey(tc.EtcdPrefix, tc.Namespace, executorID, "status")
		_, err = tc.Client.Put(ctx, statusKey, stringStatus(types.ExecutorStatusACTIVE))
		require.NoError(t, err)

		select {
		case rev, ok := <-sub:
			require.True(t, ok, "Channel should be open")
			assert.Greater(t, rev, int64(0), "Should receive a valid revision for status change")
		case <-time.After(1 * time.Second):
			t.Fatal("Should have received a notification for a status change")
		}
	}
}

func TestDeleteExecutors_Empty(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := executorStore.DeleteExecutors(ctx, tc.Namespace, []string{}, store.NopGuard())
	require.NoError(t, err)
}

// TestDeleteExecutors covers various scenarios for the DeleteExecutors method.
func TestDeleteExecutors(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Setup: Create two active executors for the tests.
	executorID1 := "executor-to-delete-1"
	executorID2 := "executor-to-delete-2"
	survivingExecutorID := "executor-survivor"
	require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, executorID1, store.HeartbeatState{Status: types.ExecutorStatusACTIVE}))
	require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, executorID2, store.HeartbeatState{Status: types.ExecutorStatusACTIVE}))
	require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, survivingExecutorID, store.HeartbeatState{Status: types.ExecutorStatusACTIVE}))

	t.Run("SucceedsForNonExistentExecutor", func(t *testing.T) {
		// Action: Delete a non-existent executor.
		err := executorStore.DeleteExecutors(ctx, tc.Namespace, []string{"non-existent-executor"}, store.NopGuard())
		// Verification: Should not return an error.
		require.NoError(t, err)
	})

	t.Run("DeletesMultipleExecutors", func(t *testing.T) {
		// Setup: Create and assign shards to multiple executors.
		execToDelete1 := "multi-delete-1"
		execToDelete2 := "multi-delete-2"
		execToKeep := "multi-keep-1"
		shardOfDeletedExecutor1 := "multi-shard-1"
		shardOfDeletedExecutor2 := "multi-shard-2"
		shardOfSurvivingExecutor := "multi-shard-keep"

		require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, execToDelete1, store.HeartbeatState{Status: types.ExecutorStatusACTIVE}))
		require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, execToDelete2, store.HeartbeatState{Status: types.ExecutorStatusACTIVE}))
		require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, execToKeep, store.HeartbeatState{Status: types.ExecutorStatusACTIVE}))

		require.NoError(t, executorStore.AssignShard(ctx, tc.Namespace, shardOfDeletedExecutor1, execToDelete1))
		require.NoError(t, executorStore.AssignShard(ctx, tc.Namespace, shardOfDeletedExecutor2, execToDelete2))
		require.NoError(t, executorStore.AssignShard(ctx, tc.Namespace, shardOfSurvivingExecutor, execToKeep))

		// Action: Delete two of the three executors in one call.
		err := executorStore.DeleteExecutors(ctx, tc.Namespace, []string{execToDelete1, execToDelete2}, store.NopGuard())
		require.NoError(t, err)

		// Verification:
		// 1. Check deleted executors are gone.
		_, _, err = executorStore.GetHeartbeat(ctx, tc.Namespace, execToDelete1)
		assert.ErrorIs(t, err, store.ErrExecutorNotFound, "Executor 1 should be gone")

		_, _, err = executorStore.GetHeartbeat(ctx, tc.Namespace, execToDelete2)
		assert.ErrorIs(t, err, store.ErrExecutorNotFound, "Executor 2 should be gone")

		// 2. Check that the surviving executor remain.
		_, _, err = executorStore.GetHeartbeat(ctx, tc.Namespace, execToKeep)
		assert.NoError(t, err, "Surviving executor should still exist")
	})
}

func TestParseExecutorKey_Errors(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)

	_, _, err := etcdkeys.ParseExecutorKey(tc.EtcdPrefix, tc.Namespace, "/wrong/prefix/exec/heartbeat")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not have expected prefix")

	key := etcdkeys.BuildExecutorsPrefix(tc.EtcdPrefix, tc.Namespace) + "too/many/parts"
	_, _, err = etcdkeys.ParseExecutorKey(tc.EtcdPrefix, tc.Namespace, key)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected key format")
}

// TestAssignAndGetShardOwnerRoundtrip verifies the successful assignment and retrieval of a shard owner.
func TestAssignAndGetShardOwnerRoundtrip(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc).(*executorStoreImpl)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := executorStore.timeSource.Now().UTC()
	executorID := "executor-roundtrip"
	shardID := "shard-roundtrip"

	// Setup: Create an active executor.
	err := executorStore.RecordHeartbeat(ctx, tc.Namespace, executorID, store.HeartbeatState{Status: types.ExecutorStatusACTIVE})
	require.NoError(t, err)

	// 1. Assign a shard to the active executor.
	err = executorStore.AssignShard(ctx, tc.Namespace, shardID, executorID)
	require.NoError(t, err, "Should successfully assign shard to an active executor")

	// 2. Get the owner and verify it's the correct executor.
	state, err := executorStore.GetState(ctx, tc.Namespace)
	require.NoError(t, err)
	assert.Contains(t, state.ShardAssignments[executorID].AssignedShards, shardID)
	assert.Equal(t, now, state.ShardAssignments[executorID].LastUpdated)
}

// TestAssignShardErrors tests the various error conditions when assigning a shard.
func TestAssignShardErrors(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	activeExecutorID := "executor-active-errors"
	drainingExecutorID := "executor-draining-errors"
	shardID1 := "shard-err-1"
	shardID2 := "shard-err-2"

	// Setup: Create an active and a draining executor, and assign one shard.
	err := executorStore.RecordHeartbeat(ctx, tc.Namespace, activeExecutorID, store.HeartbeatState{Status: types.ExecutorStatusACTIVE})
	require.NoError(t, err)
	err = executorStore.RecordHeartbeat(ctx, tc.Namespace, drainingExecutorID, store.HeartbeatState{Status: types.ExecutorStatusDRAINING})
	require.NoError(t, err)
	err = executorStore.AssignShard(ctx, tc.Namespace, shardID1, activeExecutorID)
	require.NoError(t, err)

	// Case 1: Assigning an already-assigned shard.
	err = executorStore.AssignShard(ctx, tc.Namespace, shardID1, activeExecutorID)
	require.Error(t, err, "Should fail to assign an already-assigned shard")
	var alreadyAssigned *store.ErrShardAlreadyAssigned
	require.ErrorAs(t, err, &alreadyAssigned)
	assert.Equal(t, shardID1, alreadyAssigned.ShardID)
	assert.Equal(t, activeExecutorID, alreadyAssigned.AssignedTo)
	assert.NotNil(t, alreadyAssigned.Metadata)

	// Case 2: Assigning to a non-existent executor.
	err = executorStore.AssignShard(ctx, tc.Namespace, shardID2, "non-existent-executor")
	require.Error(t, err, "Should fail to assign to a non-existent executor")
	assert.ErrorIs(t, err, store.ErrExecutorNotFound, "Error should be ErrExecutorNotFound")

	// Case 3: Assigning to a non-active (draining) executor.
	err = executorStore.AssignShard(ctx, tc.Namespace, shardID2, drainingExecutorID)
	require.Error(t, err, "Should fail to assign to a draining executor")
	assert.ErrorIs(t, err, store.ErrVersionConflict, "Error should be ErrVersionConflict for non-active executor")
}

// TestShardStatisticsPersistence verifies that shard statistics are preserved on assignment
// when they already exist, and that GetState exposes them.
func TestShardStatisticsPersistence(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	executorStore.(*executorStoreImpl).cfg = &config.Config{
		LoadBalancingMode: func(namespace string) string {
			return config.LoadBalancingModeGREEDY
		},
		MaxEtcdTxnOps: dynamicproperties.GetIntPropertyFn(128),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	executorID := "exec-stats"
	shardID := "shard-stats"

	// 1. Setup: ensure executor is ACTIVE
	require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, executorID, store.HeartbeatState{Status: types.ExecutorStatusACTIVE}))

	// 2. Pre-create shard statistics as if coming from prior history
	stats := store.ShardStatistics{SmoothedLoad: 12.5, LastUpdateTime: time.Unix(1234, 0).UTC(), LastMoveTime: time.Unix(5678, 0).UTC()}
	// The stats the executor should have after assignment
	executorStats := map[string]etcdtypes.ShardStatistics{
		shardID: *etcdtypes.FromShardStatistics(&stats),
	}
	payload, err := json.Marshal(executorStats)
	require.NoError(t, err)
	statsKey := etcdkeys.BuildExecutorKey(tc.EtcdPrefix, tc.Namespace, executorID, etcdkeys.ExecutorShardStatisticsKey)
	writer, err := common.NewRecordWriter(tc.Compression)
	require.NoError(t, err)
	compressedPayload, err := writer.Write(payload)
	require.NoError(t, err)
	// Write those stats to etcd under the executor (exec-stats) stats key
	_, err = tc.Client.Put(ctx, statsKey, string(compressedPayload))
	require.NoError(t, err)

	// 3. Assign the shard via AssignShard (should not clobber existing metrics)
	require.NoError(t, executorStore.AssignShard(ctx, tc.Namespace, shardID, executorID))

	// 4. Verify via GetState that metrics are preserved and exposed
	nsState, err := executorStore.GetState(ctx, tc.Namespace)
	require.NoError(t, err)
	require.Contains(t, nsState.ShardStats, shardID)
	updatedStats := nsState.ShardStats[shardID]
	assert.Equal(t, stats.SmoothedLoad, updatedStats.SmoothedLoad)
	assert.Equal(t, stats.LastUpdateTime, updatedStats.LastUpdateTime)
	// This should be greater than the last move time
	assert.Greater(t, updatedStats.LastMoveTime, stats.LastMoveTime)

	// 5. Also ensure assignment recorded correctly
	require.Contains(t, nsState.ShardAssignments[executorID].AssignedShards, shardID)
}

// TestGetShardStatisticsForMissingShard verifies GetState does not report statistics for unknown shards.
func TestGetShardStatisticsForMissingShard(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// No metrics are written; GetState should not contain unknown shard
	st, err := executorStore.GetState(ctx, tc.Namespace)
	require.NoError(t, err)
	assert.NotContains(t, st.ShardStats, "unknown")
}

// TestDeleteShardStatsDeletesAllStats verifies that shard statistics are correctly deleted.
func TestDeleteShardStatsDeletesAllStats(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	setLoadBalancingMode(executorStore, config.LoadBalancingModeGREEDY)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	totalShardStats := 135 // number of stats to add and make stale
	shardIDs := make([]string, 0, totalShardStats)
	executorID := "exec-delete-stats"

	// ensure executor exists
	ctxHeartbeat, cancelHb := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelHb()
	require.NoError(t, executorStore.RecordHeartbeat(ctxHeartbeat, tc.Namespace, executorID, store.HeartbeatState{Status: types.ExecutorStatusACTIVE}))

	// Create stale stats
	executorStats := make(map[string]etcdtypes.ShardStatistics)
	for i := 0; i < totalShardStats; i++ {
		shardID := "stale-stats-" + strconv.Itoa(i)
		shardIDs = append(shardIDs, shardID)

		stats := store.ShardStatistics{
			SmoothedLoad:   float64(i),
			LastUpdateTime: time.Unix(int64(i), 0).UTC(),
			LastMoveTime:   time.Unix(int64(i), 0).UTC(),
		}
		executorStats[shardID] = *etcdtypes.FromShardStatistics(&stats)
	}

	statsKey := etcdkeys.BuildExecutorKey(tc.EtcdPrefix, tc.Namespace, executorID, etcdkeys.ExecutorShardStatisticsKey)
	payload, err := json.Marshal(executorStats)
	require.NoError(t, err)
	writer, err := common.NewRecordWriter(tc.Compression)
	require.NoError(t, err)
	compressedPayload, err := writer.Write(payload)
	require.NoError(t, err)
	_, err = tc.Client.Put(ctx, statsKey, string(compressedPayload))
	require.NoError(t, err)

	require.NoError(t, executorStore.DeleteShardStats(ctx, tc.Namespace, shardIDs, store.NopGuard()))

	nsState, err := executorStore.GetState(ctx, tc.Namespace)
	require.NoError(t, err)
	// All stats should be deleted
	assert.Empty(t, nsState.ShardStats)
}

// --- Test Setup ---

func stringStatus(s types.ExecutorStatus) string {
	res, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(res)
}

func recordHeartbeats(ctx context.Context, t *testing.T, executorStore store.Store, namespace string, executorIDs ...string) {
	t.Helper()

	for _, executorID := range executorIDs {
		require.NoError(t, executorStore.RecordHeartbeat(ctx, namespace, executorID, store.HeartbeatState{Status: types.ExecutorStatusACTIVE}))
	}
}

func setLoadBalancingMode(executorStore store.Store, mode string) {
	impl := executorStore.(*executorStoreImpl)
	if impl.cfg == nil {
		impl.cfg = &config.Config{}
	}
	impl.cfg.LoadBalancingMode = func(string) string { return mode }
}

func setLoadSmoothingTimeConstant(executorStore store.Store, value time.Duration) {
	impl := executorStore.(*executorStoreImpl)
	if impl.cfg == nil {
		impl.cfg = &config.Config{}
	}
	impl.cfg.LoadBalancingGreedy.LoadSmoothingTimeConstant = func(string) time.Duration { return value }
}

// trackingTxn implements clientv3.Txn to record operations per batch for testing.
type trackingTxn struct {
	opsCount int
	commitFn func(numOps int) (*clientv3.TxnResponse, error)
}

func (t *trackingTxn) If(_ ...clientv3.Cmp) clientv3.Txn    { return t }
func (t *trackingTxn) Else(_ ...clientv3.Op) clientv3.Txn   { return t }
func (t *trackingTxn) Then(ops ...clientv3.Op) clientv3.Txn { t.opsCount += len(ops); return t }
func (t *trackingTxn) Commit() (*clientv3.TxnResponse, error) {
	return t.commitFn(t.opsCount)
}

func TestCommitOps_Batching(t *testing.T) {
	const testMaxTxnOps = 128
	const testMaxOpsPerBatch = testMaxTxnOps - guardOpOverhead

	tests := []struct {
		name            string
		numOps          int
		expectedBatches int
	}{
		{
			name:            "ZeroOps",
			numOps:          0,
			expectedBatches: 0,
		},
		{
			name:            "SingleOp",
			numOps:          1,
			expectedBatches: 1,
		},
		{
			name:            "BelowLimit",
			numOps:          50,
			expectedBatches: 1,
		},
		{
			name:            "ExactlyAtLimit",
			numOps:          testMaxOpsPerBatch,
			expectedBatches: 1,
		},
		{
			name:            "OneOverLimit",
			numOps:          testMaxOpsPerBatch + 1,
			expectedBatches: 2,
		},
		{
			name:            "ExactlyTwoBatches",
			numOps:          testMaxOpsPerBatch * 2,
			expectedBatches: 2,
		},
		{
			name:            "MultipleBatches",
			numOps:          testMaxOpsPerBatch*3 + 10,
			expectedBatches: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockClient := etcdclient.NewMockClient(ctrl)

			var batchSizes []int

			ops := make([]clientv3.Op, tt.numOps)
			for i := range ops {
				ops[i] = clientv3.OpDelete(fmt.Sprintf("/test/key/%d", i))
			}

			for range tt.expectedBatches {
				txn := &trackingTxn{
					commitFn: func(numOps int) (*clientv3.TxnResponse, error) {
						batchSizes = append(batchSizes, numOps)
						return &clientv3.TxnResponse{Succeeded: true}, nil
					},
				}
				mockClient.EXPECT().Txn(gomock.Any()).Return(txn)
			}

			s := &executorStoreImpl{
				client: mockClient,
				cfg:    &config.Config{MaxEtcdTxnOps: dynamicproperties.GetIntPropertyFn(testMaxTxnOps)},
			}
			responses, err := s.commitOps(context.Background(), ops, store.NopGuard())
			require.NoError(t, err)
			assert.Len(t, responses, tt.expectedBatches, "one response per committed batch")

			require.Len(t, batchSizes, tt.expectedBatches)

			totalOps := 0
			for _, size := range batchSizes {
				assert.LessOrEqual(t, size, testMaxOpsPerBatch)
				totalOps += size
			}
			assert.Equal(t, tt.numOps, totalOps)
		})
	}
}

func TestCommitOps_CommitError_StopsEarly(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := etcdclient.NewMockClient(ctrl)

	commitCount := 0
	makeTxn := func(err error) *trackingTxn {
		return &trackingTxn{
			commitFn: func(numOps int) (*clientv3.TxnResponse, error) {
				commitCount++
				if err != nil {
					return nil, err
				}
				return &clientv3.TxnResponse{Succeeded: true}, nil
			},
		}
	}

	// Use a small limit so we get multiple batches with fewer ops
	const testMaxTxnOps = 10
	ops := make([]clientv3.Op, testMaxTxnOps+5)
	for i := range ops {
		ops[i] = clientv3.OpDelete(fmt.Sprintf("/test/key/%d", i))
	}

	mockClient.EXPECT().Txn(gomock.Any()).Return(makeTxn(fmt.Errorf("etcd unavailable")))

	s := &executorStoreImpl{
		client: mockClient,
		cfg:    &config.Config{MaxEtcdTxnOps: dynamicproperties.GetIntPropertyFn(testMaxTxnOps)},
	}
	_, err := s.commitOps(context.Background(), ops, store.NopGuard())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "commit batch")
	assert.Equal(t, 1, commitCount, "should stop after first failing batch")
}

func TestCommitOps_LeadershipLost_StopsEarly(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := etcdclient.NewMockClient(ctrl)

	commitCount := 0
	makeTxn := func(succeeded bool) *trackingTxn {
		return &trackingTxn{
			commitFn: func(numOps int) (*clientv3.TxnResponse, error) {
				commitCount++
				return &clientv3.TxnResponse{Succeeded: succeeded}, nil
			},
		}
	}

	const testMaxTxnOps = 10
	ops := make([]clientv3.Op, testMaxTxnOps+5)
	for i := range ops {
		ops[i] = clientv3.OpDelete(fmt.Sprintf("/test/key/%d", i))
	}

	mockClient.EXPECT().Txn(gomock.Any()).Return(makeTxn(false))

	s := &executorStoreImpl{
		client: mockClient,
		cfg:    &config.Config{MaxEtcdTxnOps: dynamicproperties.GetIntPropertyFn(testMaxTxnOps)},
	}
	_, err := s.commitOps(context.Background(), ops, store.NopGuard())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "leadership may have changed")
	assert.Equal(t, 1, commitCount, "should stop after first leadership failure")
}

func TestCommitOps_GuardError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := etcdclient.NewMockClient(ctrl)

	txn := &trackingTxn{
		commitFn: func(numOps int) (*clientv3.TxnResponse, error) {
			t.Fatal("should not reach commit")
			return nil, nil
		},
	}
	mockClient.EXPECT().Txn(gomock.Any()).Return(txn)

	failingGuard := func(txn store.Txn) (store.Txn, error) {
		return nil, fmt.Errorf("guard failure")
	}

	ops := []clientv3.Op{clientv3.OpDelete("/test/key")}
	s := &executorStoreImpl{
		client: mockClient,
		cfg:    &config.Config{MaxEtcdTxnOps: dynamicproperties.GetIntPropertyFn(128)},
	}
	_, err := s.commitOps(context.Background(), ops, failingGuard)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "apply transaction guard")
}

// The guard is applied to every batch, not just the first.
// A leadership change part-way through a large write must stop the remaining batches.
func TestCommitOps_GuardAppliedPerBatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := etcdclient.NewMockClient(ctrl)

	const testMaxTxnOps = 10
	ops := make([]clientv3.Op, testMaxTxnOps+5)
	for i := range ops {
		ops[i] = clientv3.OpDelete(fmt.Sprintf("/test/key/%d", i))
	}

	for range 2 {
		txn := &trackingTxn{
			commitFn: func(numOps int) (*clientv3.TxnResponse, error) {
				return &clientv3.TxnResponse{Succeeded: true}, nil
			},
		}
		mockClient.EXPECT().Txn(gomock.Any()).Return(txn)
	}

	guardCalls := 0
	countingGuard := func(txn store.Txn) (store.Txn, error) {
		guardCalls++
		return txn, nil
	}

	s := &executorStoreImpl{
		client: mockClient,
		cfg:    &config.Config{MaxEtcdTxnOps: dynamicproperties.GetIntPropertyFn(testMaxTxnOps)},
	}
	responses, err := s.commitOps(context.Background(), ops, countingGuard)

	require.NoError(t, err)
	assert.Len(t, responses, 2)
	assert.Equal(t, 2, guardCalls, "each batch gets its own guarded transaction")
}

// UndrainShards maps per-op delete counts back to its input by walking the returned
// responses in order, so the responses must line up with how the ops were submitted.
func TestCommitOps_ResponsesFollowSubmissionOrder(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := etcdclient.NewMockClient(ctrl)

	const testMaxTxnOps = 4
	ops := make([]clientv3.Op, 7)
	for i := range ops {
		ops[i] = clientv3.OpDelete(fmt.Sprintf("/test/key/%d", i))
	}

	// Each batch reports its own op count as a delete count, so the flattened
	// sequence of responses reveals the order the batches were committed in.
	for range 3 {
		txn := &trackingTxn{
			commitFn: func(numOps int) (*clientv3.TxnResponse, error) {
				batch := make([]*etcdserverpb.ResponseOp, 0, numOps)
				for i := range numOps {
					batch = append(batch, &etcdserverpb.ResponseOp{
						Response: &etcdserverpb.ResponseOp_ResponseDeleteRange{
							ResponseDeleteRange: &etcdserverpb.DeleteRangeResponse{Deleted: int64(i)},
						},
					})
				}
				return &clientv3.TxnResponse{Succeeded: true, Responses: batch}, nil
			},
		}
		mockClient.EXPECT().Txn(gomock.Any()).Return(txn)
	}

	s := &executorStoreImpl{
		client: mockClient,
		cfg:    &config.Config{MaxEtcdTxnOps: dynamicproperties.GetIntPropertyFn(testMaxTxnOps)},
	}
	responses, err := s.commitOps(context.Background(), ops, store.NopGuard())
	require.NoError(t, err)

	var deleted []int64
	for _, resp := range responses {
		for _, opResp := range resp.Responses {
			deleted = append(deleted, opResp.GetResponseDeleteRange().Deleted)
		}
	}
	assert.Equal(t, []int64{0, 1, 2, 0, 1, 2, 0}, deleted,
		"every submitted op has exactly one response, in submission order")
}

// TestResetNamespace verifies that ResetNamespace removes every key under the
// namespace prefix in a single etcd op, regardless of the executor topology.
func TestResetNamespace(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("EmptyNamespace_NoError_NoDeletions", func(t *testing.T) {
		emptyNs := tc.Namespace + "-empty"
		deleted, err := executorStore.ResetNamespace(ctx, emptyNs)
		require.NoError(t, err)
		assert.Equal(t, int64(0), deleted)
	})

	t.Run("DeletesAllExecutorKeys_LeavesOtherNamespacesIntact", func(t *testing.T) {
		// Seed two executors in the target namespace plus assignments.
		executorA := "reset-exec-a"
		executorB := "reset-exec-b"
		require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, executorA, store.HeartbeatState{
			Status:   types.ExecutorStatusACTIVE,
			Metadata: map[string]string{"zone": "dca1"},
		}))
		require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, executorB, store.HeartbeatState{
			Status: types.ExecutorStatusACTIVE,
		}))
		require.NoError(t, executorStore.AssignShard(ctx, tc.Namespace, "shard-1", executorA))
		require.NoError(t, executorStore.AssignShard(ctx, tc.Namespace, "shard-2", executorB))

		// Seed an executor in a sibling namespace that must not be touched.
		siblingNs := tc.Namespace + "-sibling"
		siblingExec := "sibling-exec"
		require.NoError(t, executorStore.RecordHeartbeat(ctx, siblingNs, siblingExec, store.HeartbeatState{
			Status: types.ExecutorStatusACTIVE,
		}))

		// Sanity-check the seed.
		state, err := executorStore.GetState(ctx, tc.Namespace)
		require.NoError(t, err)
		require.Len(t, state.Executors, 2)

		deleted, err := executorStore.ResetNamespace(ctx, tc.Namespace)
		require.NoError(t, err)
		assert.Greater(t, deleted, int64(0), "should report a positive delete count")

		// Target namespace is empty. Trailing slash mirrors the delete-side
		// convention so we don't accidentally count sibling-namespace keys
		// when one namespace name is a prefix substring of another.
		afterPrefix := etcdkeys.BuildNamespacePrefix(tc.EtcdPrefix, tc.Namespace)
		resp, err := tc.Client.Get(ctx, afterPrefix, clientv3.WithPrefix())
		require.NoError(t, err)
		assert.Equal(t, int64(0), resp.Count, "no keys should remain under namespace prefix")

		// Sibling namespace is untouched.
		siblingPrefix := etcdkeys.BuildNamespacePrefix(tc.EtcdPrefix, siblingNs)
		respSibling, err := tc.Client.Get(ctx, siblingPrefix, clientv3.WithPrefix())
		require.NoError(t, err)
		assert.Greater(t, respSibling.Count, int64(0), "sibling namespace must not be wiped")
	})
}

// TestDrainShardsLifecycle walks the drain/undrain lifecycle through the etcd-backed
// store: reading an empty set, draining, reading back via both GetDrainedShards and
// GetState, draining idempotently, and undraining.
func TestDrainShardsLifecycle(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	drained, err := executorStore.GetDrainedShards(ctx, tc.Namespace)
	require.NoError(t, err)
	assert.Empty(t, drained, "no shards are drained before the first DrainShards call")

	require.NoError(t, executorStore.DrainShards(ctx, tc.Namespace, []string{"shard-B", "shard-A"}))

	drained, err = executorStore.GetDrainedShards(ctx, tc.Namespace)
	require.NoError(t, err)
	assert.Equal(t, []string{"shard-A", "shard-B"}, drained, "the drained set reads back sorted")

	// Draining an already-drained shard alongside a new one is idempotent: the
	// existing shard stays drained instead of erroring.
	require.NoError(t, executorStore.DrainShards(ctx, tc.Namespace, []string{"shard-A", "shard-C"}))

	drained, err = executorStore.GetDrainedShards(ctx, tc.Namespace)
	require.NoError(t, err)
	assert.Equal(t, []string{"shard-A", "shard-B", "shard-C"}, drained)

	state, err := executorStore.GetState(ctx, tc.Namespace)
	require.NoError(t, err)
	assert.Equal(t, map[string]struct{}{
		"shard-A": {},
		"shard-B": {},
		"shard-C": {},
	}, state.DrainedShards, "GetState exposes the same set to the rebalance loop")

	removed, err := executorStore.UndrainShards(ctx, tc.Namespace, []string{"shard-A", "shard-B"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"shard-A", "shard-B"}, removed)

	drained, err = executorStore.GetDrainedShards(ctx, tc.Namespace)
	require.NoError(t, err)
	assert.Equal(t, []string{"shard-C"}, drained)
}

// UndrainShards reports only the shards it actually removed. A shard that was never
// drained, or that a previous call already removed, is excluded — that is what makes
// the result meaningful to an operator rather than an echo of the request.
func TestUndrainShardsReportsOnlyActualRemovals(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, executorStore.DrainShards(ctx, tc.Namespace, []string{"shard-A"}))

	removed, err := executorStore.UndrainShards(ctx, tc.Namespace, []string{"shard-A", "never-drained"})
	require.NoError(t, err)
	assert.Equal(t, []string{"shard-A"}, removed, "never-drained was absent, so it is not reported")

	removed, err = executorStore.UndrainShards(ctx, tc.Namespace, []string{"shard-A", "never-drained"})
	require.NoError(t, err)
	assert.Empty(t, removed, "repeating the same undrain removes nothing further")
}

// Draining must not leak across namespaces: the drained keyspace is per-namespace and
// a prefix scan for one namespace must not observe another's keys.
func TestDrainShardsIsolatedPerNamespace(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	otherNamespace := tc.Namespace + "-other"

	require.NoError(t, executorStore.DrainShards(ctx, tc.Namespace, []string{"shard-A"}))
	require.NoError(t, executorStore.DrainShards(ctx, otherNamespace, []string{"shard-Z"}))

	drained, err := executorStore.GetDrainedShards(ctx, tc.Namespace)
	require.NoError(t, err)
	assert.Equal(t, []string{"shard-A"}, drained)

	drained, err = executorStore.GetDrainedShards(ctx, otherNamespace)
	require.NoError(t, err)
	assert.Equal(t, []string{"shard-Z"}, drained)
}

// Draining more shards than MaxEtcdTxnOps must transparently chunk the transactions
// rather than fail with "etcdserver: too many operations in txn request". Undrain has
// to map per-op delete counts back to inputs correctly across those chunk boundaries.
func TestDrainShardsChunksOverTxnLimit(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const numShards = 300 // more than 2x the configured MaxEtcdTxnOps of 128
	shardIDs := make([]string, 0, numShards)
	for i := 0; i < numShards; i++ {
		shardIDs = append(shardIDs, fmt.Sprintf("bulk-shard-%04d", i))
	}

	require.NoError(t, executorStore.DrainShards(ctx, tc.Namespace, shardIDs))

	drained, err := executorStore.GetDrainedShards(ctx, tc.Namespace)
	require.NoError(t, err)
	assert.ElementsMatch(t, shardIDs, drained)

	removed, err := executorStore.UndrainShards(ctx, tc.Namespace, shardIDs)
	require.NoError(t, err)
	assert.ElementsMatch(t, shardIDs, removed, "every shard is attributed to the right op across chunks")

	drained, err = executorStore.GetDrainedShards(ctx, tc.Namespace)
	require.NoError(t, err)
	assert.Empty(t, drained)
}

func TestDrainShardsEmptyInput(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, executorStore.DrainShards(ctx, tc.Namespace, nil))

	removed, err := executorStore.UndrainShards(ctx, tc.Namespace, nil)
	require.NoError(t, err)
	assert.Empty(t, removed)

	drained, err := executorStore.GetDrainedShards(ctx, tc.Namespace)
	require.NoError(t, err)
	assert.Empty(t, drained)
}

// A shard ID that cannot round-trip through a key must be rejected at write time.
// Writing it would succeed but every read would skip it, leaving a drained shard
// that no API can report or undrain.
func TestDrainUndrainRejectUnrepresentableShardIDs(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for name, shardID := range map[string]string{
		"empty":          "",
		"slash":          "shard/A",
		"trailing slash": "shard-A/",
		"nested path":    "a/b/c",
	} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, executorStore.DrainShards(ctx, tc.Namespace, []string{shardID}))
			_, err := executorStore.UndrainShards(ctx, tc.Namespace, []string{shardID})
			require.Error(t, err)
		})
	}

	// The rejection happens before any write, so a bad ID alongside a good one
	// leaves nothing drained rather than partially applying.
	require.Error(t, executorStore.DrainShards(ctx, tc.Namespace, []string{"shard-A", "bad/id"}))

	drained, err := executorStore.GetDrainedShards(ctx, tc.Namespace)
	require.NoError(t, err)
	assert.Empty(t, drained, "a rejected batch must not write its valid shards")
}

// ResetNamespace wipes the whole namespace prefix, so drained-shard keys must go with
// it. Otherwise a reset namespace would come back with stale shards still blocked.
func TestResetNamespaceClearsDrainedShards(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, executorStore.DrainShards(ctx, tc.Namespace, []string{"shard-A", "shard-B"}))

	_, err := executorStore.ResetNamespace(ctx, tc.Namespace)
	require.NoError(t, err)

	drained, err := executorStore.GetDrainedShards(ctx, tc.Namespace)
	require.NoError(t, err)
	assert.Empty(t, drained)
}

// A key that does not parse as a drained-shard key is skipped rather than failing the
// read, so one malformed key cannot stall GetState and with it the rebalance loop.
func TestLoadDrainedShardSetSkipsMalformedKeys(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	executorStore := createStore(t, tc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, executorStore.DrainShards(ctx, tc.Namespace, []string{"shard-A"}))

	// Nested key: within the drained prefix but not a valid single-segment shard ID.
	// Only reachable by writing directly to etcd now that DrainShards validates input.
	malformed := etcdkeys.BuildDrainedShardKey(tc.EtcdPrefix, tc.Namespace, "shard-B") + "/nested"
	_, err := tc.Client.Put(ctx, malformed, "")
	require.NoError(t, err)

	drained, err := executorStore.GetDrainedShards(ctx, tc.Namespace)
	require.NoError(t, err)
	assert.Equal(t, []string{"shard-A"}, drained)

	// GetState reads the same keyspace through the transaction path, so it must
	// tolerate the malformed key too.
	state, err := executorStore.GetState(ctx, tc.Namespace)
	require.NoError(t, err)
	assert.Equal(t, map[string]struct{}{"shard-A": {}}, state.DrainedShards)
}

func createStore(t *testing.T, tc *testhelper.StoreTestCluster) store.Store {
	t.Helper()

	etcdConfig, err := etcdclient.NewExecutorStoreConfig(tc.SDConfig)
	require.NoError(t, err)

	store, err := NewStore(ExecutorStoreParams{
		Client:        tc.Client,
		ETCDConfig:    etcdConfig,
		Lifecycle:     fxtest.NewLifecycle(t),
		Logger:        testlogger.New(t),
		TimeSource:    clock.NewMockedTimeSourceAt(time.Now()),
		MetricsClient: metrics.NewNoopMetricsClient(),
		Config: &config.Config{
			LoadBalancingMode: func(namespace string) string { return config.LoadBalancingModeNAIVE },
			MaxEtcdTxnOps:     dynamicproperties.GetIntPropertyFn(128),
			LoadBalancingGreedy: config.LoadBalancingGreedyConfig{
				LoadSmoothingTimeConstant: func(string) time.Duration { return statistics.DefaultLoadSmoothingTimeConstant },
			},
		},
	})
	require.NoError(t, err)
	return store
}
