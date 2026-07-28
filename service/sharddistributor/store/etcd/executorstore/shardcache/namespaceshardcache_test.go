package shardcache

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/api/v3/etcdserverpb"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdclient"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdkeys"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdtypes"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/executorstore/common"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/testhelper"
)

func TestNamespaceShardToExecutor_Lifecycle(t *testing.T) {
	testCluster := testhelper.SetupStoreTestCluster(t)
	logger := testlogger.New(t)
	stopCh := make(chan struct{})
	defer close(stopCh)

	// Setup: Create executor-1 with shard-1
	setupExecutorWithShards(t, testCluster, "executor-1", []string{"shard-1"}, map[string]string{
		"hostname": "executor-1-host",
		"version":  "v1.0.0",
	})

	// Start the cache
	namespaceShardToExecutor, err := newNamespaceShardToExecutor(testCluster.EtcdPrefix, testCluster.Namespace, testCluster.Client, stopCh, logger, clock.NewRealTimeSource(), metrics.NewNoopMetricsClient())
	assert.NoError(t, err)
	namespaceShardToExecutor.Start(&sync.WaitGroup{})
	time.Sleep(50 * time.Millisecond)

	// Verify executor-1 owns shard-1 with correct metadata
	verifyShardOwner(t, namespaceShardToExecutor, "shard-1", "executor-1", map[string]string{
		"hostname": "executor-1-host",
		"version":  "v1.0.0",
	})

	// Check the cache is populated
	namespaceShardToExecutor.RLock()
	_, ok := namespaceShardToExecutor.executorRevision["executor-1"]
	assert.True(t, ok)
	assert.Equal(t, "executor-1", namespaceShardToExecutor.shardToExecutor["shard-1"].ExecutorID)
	namespaceShardToExecutor.RUnlock()

	// Add executor-2 with shard-2 to trigger watch update
	setupExecutorWithShards(t, testCluster, "executor-2", []string{"shard-2"}, map[string]string{
		"hostname": "executor-2-host",
		"region":   "us-west",
	})
	time.Sleep(100 * time.Millisecond)

	// Check that executor-2 and shard-2 is in the cache
	namespaceShardToExecutor.RLock()
	_, ok = namespaceShardToExecutor.executorRevision["executor-2"]
	assert.True(t, ok)
	assert.Equal(t, "executor-2", namespaceShardToExecutor.shardToExecutor["shard-2"].ExecutorID)
	namespaceShardToExecutor.RUnlock()

	// Verify executor-2 owns shard-2 with correct metadata
	verifyShardOwner(t, namespaceShardToExecutor, "shard-2", "executor-2", map[string]string{
		"hostname": "executor-2-host",
		"region":   "us-west",
	})
}

func TestNamespaceShardToExecutor_Subscribe(t *testing.T) {
	testCluster := testhelper.SetupStoreTestCluster(t)
	logger := testlogger.New(t)
	stopCh := make(chan struct{})
	defer close(stopCh)

	// Setup: Create executor-1 with shard-1
	setupExecutorWithShards(t, testCluster, "executor-1", []string{"shard-1"}, map[string]string{
		"hostname": "executor-1-host",
		"version":  "v1.0.0",
	})

	// Start the cache
	namespaceShardToExecutor, err := newNamespaceShardToExecutor(testCluster.EtcdPrefix, testCluster.Namespace, testCluster.Client, stopCh, logger, clock.NewRealTimeSource(), metrics.NewNoopMetricsClient())
	assert.NoError(t, err)
	namespaceShardToExecutor.Start(&sync.WaitGroup{})

	// Refresh the cache to get the initial state
	err = namespaceShardToExecutor.refresh(context.Background())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	subCh, unSub := namespaceShardToExecutor.Subscribe(ctx)
	defer unSub()

	var wg sync.WaitGroup
	wg.Add(1)

	// start listener
	go func() {
		defer wg.Done()
		// Check that we get the initial state
		state := <-subCh
		assert.Len(t, state, 1)
		verifyExecutorInState(t, state, "executor-1", []string{"shard-1"}, map[string]string{
			"hostname": "executor-1-host",
			"version":  "v1.0.0",
		})

		// Check that we get the updated state
		state = <-subCh
		assert.Len(t, state, 2)
		verifyExecutorInState(t, state, "executor-1", []string{"shard-1"}, map[string]string{
			"hostname": "executor-1-host",
			"version":  "v1.0.0",
		})
		verifyExecutorInState(t, state, "executor-2", []string{"shard-2"}, map[string]string{
			"hostname": "executor-2-host",
			"region":   "us-west",
		})
	}()
	time.Sleep(10 * time.Millisecond)

	// Add executor-2 with shard-2 to trigger new subscription update
	setupExecutorWithShards(t, testCluster, "executor-2", []string{"shard-2"}, map[string]string{
		"hostname": "executor-2-host",
		"region":   "us-west",
	})

	wg.Wait()
}

func TestNamespaceShardToExecutor_watch_watchChanErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := testlogger.New(t)
	mockClient := etcdclient.NewMockClient(ctrl)
	stopCh := make(chan struct{})
	testPrefix := "/test-prefix"
	testNamespace := "test-namespace"

	// Mock the Watch call to return our watch channel
	watchChan := make(chan clientv3.WatchResponse)
	mockClient.EXPECT().
		Watch(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(watchChan).
		AnyTimes()

	e, err := newNamespaceShardToExecutor(testPrefix, testNamespace, mockClient, stopCh, logger, clock.NewRealTimeSource(), metrics.NewNoopMetricsClient())
	require.NoError(t, err)

	triggerChan := make(chan struct{}, 1)

	// Test Case #1
	// Test received compact revision error from watch channel
	{
		go func() {
			watchChan <- clientv3.WatchResponse{
				CompactRevision: 100,
			}
		}()

		err = e.watch(triggerChan)
		require.Error(t, err)
		assert.ErrorContains(t, err, "etcdserver: mvcc: required revision has been compacted")
	}

	// Test Case #2
	// Test closed watch channel
	{
		close(watchChan)
		err = e.watch(triggerChan)
		require.Error(t, err)
		assert.ErrorContains(t, err, "watch channel closed")
	}
}

func TestNamespaceShardToExecutor_watch_triggerChBlocking(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer tc.ctrl.Finish()
	defer goleak.VerifyNone(t)

	// Create a triggerCh with buffer size 1, but never read from it
	triggerChan := make(chan struct{}, 1)

	executorKey := etcdkeys.BuildExecutorKey(tc.prefix, tc.namespace, tc.executorID, etcdkeys.ExecutorAssignedStateKey)

	// Start watch in a goroutine
	watchDone := make(chan error, 1)
	go func() {
		watchDone <- tc.e.watch(triggerChan)
	}()

	// Send many events - the loop should not block even though triggerCh is full
	for i := 0; i < 100; i++ {
		select {
		case tc.watchChan <- clientv3.WatchResponse{
			Events: []*clientv3.Event{
				{
					Type: clientv3.EventTypePut,
					Kv: &mvccpb.KeyValue{
						Key: []byte(executorKey),
					},
				},
			},
		}:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("watch loop is stuck - could not send event to watchChan")
		}
	}

	// Close stopCh to exit the watch loop
	close(tc.stopCh)

	select {
	case err := <-watchDone:
		assert.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Fatal("watch loop did not exit after stopCh was closed")
	}
}

func TestNamespaceShardToExecutor_namespaceRefreshLoop_notTriggersRefresh_reportedShards(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer tc.ctrl.Finish()
	defer goleak.VerifyNone(t)

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		tc.e.namespaceRefreshLoop()
	}()

	key := etcdkeys.BuildExecutorKey(
		tc.prefix,
		tc.namespace,
		tc.executorID,
		etcdkeys.ExecutorReportedShardsKey,
	)

	tc.watchChan <- clientv3.WatchResponse{
		Events: []*clientv3.Event{
			{
				Type: clientv3.EventTypePut,
				Kv: &mvccpb.KeyValue{
					Key: []byte(key),
				},
			},
		},
	}

	// mock for refresh should not be called, so no need to set expectation on etcdClient.EXPECT().Get()
	// use Never with condition that checks shardOwners is still empty to verify that refresh is not triggered
	require.Neverf(t, func() bool {
		tc.e.RLock()
		defer tc.e.RUnlock()

		return len(tc.e.shardOwners) > 0
	}, 100*time.Millisecond, 1*time.Millisecond, "expected no refresh to be triggered for reported shards change")

	// Close stopCh to exit the loop
	close(tc.stopCh)
	wg.Wait()
}

func TestNamespaceShardToExecutor_namespaceRefreshLoop_notTriggersRefresh_noUpdates(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer tc.ctrl.Finish()
	defer goleak.VerifyNone(t)

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		tc.e.namespaceRefreshLoop()
	}()

	metadataValue := "metadata-value"
	metadataKey := "metadata-key"
	key := etcdkeys.BuildMetadataKey(
		tc.prefix,
		tc.namespace,
		tc.executorID,
		metadataKey,
	)

	tc.watchChan <- clientv3.WatchResponse{
		Events: []*clientv3.Event{
			{
				Type: clientv3.EventTypePut,
				Kv: &mvccpb.KeyValue{
					Key:   []byte(key),
					Value: []byte(metadataValue),
				},
				PrevKv: &mvccpb.KeyValue{
					Key:   []byte(key),
					Value: []byte(metadataValue),
				},
			},
		},
	}

	// mock for refresh should not be called, so no need to set expectation on etcdClient.EXPECT().Get()
	// use Never with condition that checks shardOwners is still empty to verify that refresh is not triggered
	require.Neverf(t, func() bool {
		tc.e.RLock()
		defer tc.e.RUnlock()

		return len(tc.e.shardOwners) > 0
	}, 100*time.Millisecond, 1*time.Millisecond, "expected no refresh to be triggered for the same metadata value")

	// Close stopCh to exit the loop
	close(tc.stopCh)
	wg.Wait()
}

func TestNamespaceShardToExecutor_namespaceRefreshLoop_triggersRefresh(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer tc.ctrl.Finish()
	defer goleak.VerifyNone(t)

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		tc.e.namespaceRefreshLoop()
	}()

	metadataValue := "metadata-value"
	metadataKey := "metadata-key"
	key := etcdkeys.BuildMetadataKey(
		tc.prefix,
		tc.namespace,
		tc.executorID,
		metadataKey,
	)

	// Mock Get call for refresh
	tc.etcdClient.EXPECT().
		Get(gomock.Any(), tc.executorPrefix, gomock.Any()).
		Return(
			&clientv3.GetResponse{Header: &etcdserverpb.ResponseHeader{Revision: 1}, Kvs: []*mvccpb.KeyValue{
				{
					Key:   []byte(key),
					Value: []byte(metadataValue),
				},
			}},
			nil,
		)

	// Send a watch event for metadata change which should trigger the refresh
	tc.watchChan <- clientv3.WatchResponse{
		Events: []*clientv3.Event{
			{
				Type: clientv3.EventTypePut,
				Kv: &mvccpb.KeyValue{
					Key:   []byte(key),
					Value: []byte(metadataValue),
				},
				PrevKv: &mvccpb.KeyValue{
					Key:   []byte(key),
					Value: []byte("previous value"),
				},
			},
		},
	}

	// Wait for the refresh to be triggered and the shard owner to be updated with the new metadata value
	require.Eventually(t, func() bool {
		tc.e.RLock()
		defer tc.e.RUnlock()

		shardOwner, ok := tc.e.shardOwners[tc.executorID]
		if !ok {
			return false
		}

		return shardOwner.Metadata[metadataKey] == metadataValue
	}, time.Second, 1*time.Millisecond, "expected metadata value to be updated in shard owner after refresh")

	// Close stopCh to exit the loop
	close(tc.stopCh)
	wg.Wait()
}

func TestNamespaceShardToExecutor_replaceExecutorState_skipsStaleRevision(t *testing.T) {
	defer goleak.VerifyNone(t)

	logger := testlogger.New(t)
	stopCh := make(chan struct{})
	defer close(stopCh)

	ctrl := gomock.NewController(t)

	mockClient := etcdclient.NewMockClient(ctrl)

	e, err := newNamespaceShardToExecutor("prefix", "ns", mockClient, stopCh, logger, clock.NewMockedTimeSource(), metrics.NewNoopMetricsClient())
	require.NoError(t, err)

	ownerA := &store.ShardOwner{ExecutorID: "exec-a", Metadata: map[string]string{}}
	ownerB := &store.ShardOwner{ExecutorID: "exec-b", Metadata: map[string]string{}}

	// Apply revision 10
	e.replaceExecutorState(10,
		map[string]*store.ShardOwner{"shard-1": ownerA},
		map[*store.ShardOwner][]string{ownerA: {"shard-1"}},
		map[string]int64{"exec-a": 10},
		map[string]*store.ShardOwner{"exec-a": ownerA},
	)

	got := e.getExecutorState()
	require.Len(t, got, 1)

	// Apply revision 5 (stale) — should be ignored
	e.replaceExecutorState(5,
		map[string]*store.ShardOwner{"shard-2": ownerB},
		map[*store.ShardOwner][]string{ownerB: {"shard-2"}},
		map[string]int64{"exec-b": 5},
		map[string]*store.ShardOwner{"exec-b": ownerB},
	)

	got = e.getExecutorState()
	require.Len(t, got, 1)
	for owner := range got {
		assert.Equal(t, "exec-a", owner.ExecutorID)
	}

	// Apply revision 20 (newer) — should be accepted
	e.replaceExecutorState(20,
		map[string]*store.ShardOwner{"shard-2": ownerB},
		map[*store.ShardOwner][]string{ownerB: {"shard-2"}},
		map[string]int64{"exec-b": 20},
		map[string]*store.ShardOwner{"exec-b": ownerB},
	)

	got = e.getExecutorState()
	require.Len(t, got, 1)
	for owner := range got {
		assert.Equal(t, "exec-b", owner.ExecutorID)
	}
}

func TestNamespaceShardToExecutor_namespaceRefreshLoop_watchError(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := testlogger.New(t)
	mockClient := etcdclient.NewMockClient(ctrl)
	timeSource := clock.NewMockedTimeSource()
	stopCh := make(chan struct{})
	testPrefix := "/test-prefix"
	testNamespace := "test-namespace"

	// mock for first watch call that receives error
	watchChanRcvErr := make(chan clientv3.WatchResponse)
	mockClient.EXPECT().
		Watch(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(watchChanRcvErr)

	// mock for second watch call that receives closed channel
	watchChanClosed := make(chan clientv3.WatchResponse)
	mockClient.EXPECT().
		Watch(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(watchChanClosed)

	// mock for third watch call that will be used when stopCh is closed
	// maybe called or not if stopCh is closed before retry interval
	mockClient.EXPECT().
		Watch(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(make(chan clientv3.WatchResponse)).
		MinTimes(0).
		MaxTimes(1)

	e, err := newNamespaceShardToExecutor(testPrefix, testNamespace, mockClient, stopCh, logger, timeSource, metrics.NewNoopMetricsClient())
	require.NoError(t, err)

	wg := sync.WaitGroup{}
	wg.Add(1)
	finished := atomic.Bool{}

	go func() {
		defer wg.Done()
		e.namespaceRefreshLoop()
		finished.Store(true)
	}()

	// Test Case #1: watchChan receives error
	{
		// Sends a response containing compact revision to simulate error
		watchChanRcvErr <- clientv3.WatchResponse{
			CompactRevision: 100,
		}

		timeSource.BlockUntil(1)
		require.False(t, finished.Load(), "namespaceRefreshLoop should not exit on watch error")
	}

	// Test Case #2: watchChan is closed
	{
		timeSource.Advance(2 * namespaceRefreshLoopWatchRetryInterval)

		// Sends a response containing compact revision to simulate error
		close(watchChanClosed)

		timeSource.BlockUntil(1)
		require.False(t, finished.Load(), "namespaceRefreshLoop should not exit on watch error")
	}

	// Test Case #3: stopCh is closed
	{
		timeSource.Advance(2 * namespaceRefreshLoopWatchRetryInterval)

		close(stopCh)
		wg.Wait()
		require.True(t, finished.Load(), "namespaceRefreshLoop should exit on watch error")
	}
}

// setupExecutorWithShards creates an executor in etcd with assigned shards and metadata
func setupExecutorWithShards(t *testing.T, testCluster *testhelper.StoreTestCluster, executorID string, shards []string, metadata map[string]string) {
	// Create assigned state
	assignedState := &etcdtypes.AssignedState{
		AssignedShards: make(map[string]*types.ShardAssignment),
	}
	for _, shardID := range shards {
		assignedState.AssignedShards[shardID] = &types.ShardAssignment{Status: types.AssignmentStatusREADY}
	}
	assignedStateJSON, err := json.Marshal(assignedState)
	require.NoError(t, err)

	var operations []clientv3.Op

	executorAssignedStateKey := etcdkeys.BuildExecutorKey(testCluster.EtcdPrefix, testCluster.Namespace, executorID, etcdkeys.ExecutorAssignedStateKey)
	operations = append(operations, clientv3.OpPut(executorAssignedStateKey, string(assignedStateJSON)))

	// Add metadata
	for key, value := range metadata {
		metadataKey := etcdkeys.BuildMetadataKey(testCluster.EtcdPrefix, testCluster.Namespace, executorID, key)
		operations = append(operations, clientv3.OpPut(metadataKey, value))
	}

	txnResp, err := testCluster.Client.Txn(context.Background()).Then(operations...).Commit()
	require.NoError(t, err)
	require.True(t, txnResp.Succeeded)
}

func verifyExecutorInState(t *testing.T, state map[*store.ShardOwner][]string, executorID string, shards []string, metadata map[string]string) {
	executorInState := false
	for executor, executorShards := range state {
		if executor.ExecutorID == executorID {
			assert.Equal(t, shards, executorShards)
			assert.Equal(t, metadata, executor.Metadata)
			executorInState = true
			break
		}
	}
	assert.True(t, executorInState)
}

func TestNamespaceShardToExecutor_ExecutorStatistics(t *testing.T) {
	testCluster := testhelper.SetupStoreTestCluster(t)
	logger := testlogger.New(t)
	stopCh := make(chan struct{})
	defer close(stopCh)
	mockTime := clock.NewMockedTimeSource()

	executorID := "executor-stats"
	shardID1 := "shard-stats-1"
	shardID2 := "shard-stats-2"

	initialStats := map[string]etcdtypes.ShardStatistics{
		shardID1: {SmoothedLoad: 10.0, LastUpdateTime: etcdtypes.Time(mockTime.Now().UTC().Add(-time.Hour)), LastMoveTime: etcdtypes.Time(mockTime.Now().UTC().Add(-2 * time.Hour))},
		shardID2: {SmoothedLoad: 20.0, LastUpdateTime: etcdtypes.Time(mockTime.Now().UTC().Add(-30 * time.Minute)), LastMoveTime: etcdtypes.Time(mockTime.Now().UTC().Add(-90 * time.Minute))},
	}
	putExecutorStatisticsInEtcd(t, testCluster, executorID, initialStats)

	namespaceShardToExecutor, err := newNamespaceShardToExecutor(testCluster.EtcdPrefix, testCluster.Namespace, testCluster.Client, stopCh, logger, mockTime, metrics.NewNoopMetricsClient())
	assert.NoError(t, err)
	namespaceShardToExecutor.Start(&sync.WaitGroup{})

	// GetExecutorStatistics triggers refresh on cache miss, so no need to wait for timers
	statsFromCache, err := namespaceShardToExecutor.GetExecutorStatistics(context.Background(), executorID)
	require.NoError(t, err)
	assert.Equal(t, initialStats, statsFromCache)

	updatedStats := map[string]etcdtypes.ShardStatistics{
		shardID1: {SmoothedLoad: 15.0, LastUpdateTime: etcdtypes.Time(mockTime.Now().UTC())},
		shardID2: {SmoothedLoad: 25.0, LastUpdateTime: etcdtypes.Time(mockTime.Now().UTC())},
	}
	putExecutorStatisticsInEtcd(t, testCluster, executorID, updatedStats)

	// Wait for stats to be updated via watch event
	assert.Eventually(t, func() bool {
		stats, err := namespaceShardToExecutor.GetExecutorStatistics(context.Background(), executorID)
		if err != nil {
			return false
		}
		return stats[shardID1].SmoothedLoad == 15.0
	}, 5*time.Second, 50*time.Millisecond, "stats should be updated via watch")

	statsFromCacheAfterUpdate, err := namespaceShardToExecutor.GetExecutorStatistics(context.Background(), executorID)
	require.NoError(t, err)
	assert.Equal(t, updatedStats, statsFromCacheAfterUpdate)

	nonExistentStats, err := namespaceShardToExecutor.GetExecutorStatistics(context.Background(), "non-existent-executor")
	require.Error(t, err)
	assert.ErrorIs(t, err, store.ErrExecutorNotFound)
	assert.Nil(t, nonExistentStats)

	statsKey := etcdkeys.BuildExecutorKey(testCluster.EtcdPrefix, testCluster.Namespace, executorID, etcdkeys.ExecutorShardStatisticsKey)
	_, err = testCluster.Client.Delete(context.Background(), statsKey)
	require.NoError(t, err)

	// Wait for stats to be deleted via watch event, subsequent calls should error
	assert.Eventually(t, func() bool {
		_, err := namespaceShardToExecutor.GetExecutorStatistics(context.Background(), executorID)
		return err != nil && assert.ErrorIs(t, err, store.ErrExecutorNotFound)
	}, 5*time.Second, 50*time.Millisecond, "GetExecutorStatistics should return an error after deletion")
}

// putExecutorStatisticsInEtcd is a helper to directly put compressed executor statistics into etcd.
func putExecutorStatisticsInEtcd(t *testing.T, tc *testhelper.StoreTestCluster, executorID string, stats map[string]etcdtypes.ShardStatistics) {
	payload, err := json.Marshal(stats)
	require.NoError(t, err)

	writer, err := common.NewRecordWriter(tc.Compression)
	require.NoError(t, err)
	compressedPayload, err := writer.Write(payload)
	require.NoError(t, err)

	statsKey := etcdkeys.BuildExecutorKey(tc.EtcdPrefix, tc.Namespace, executorID, etcdkeys.ExecutorShardStatisticsKey)
	_, err = tc.Client.Put(context.Background(), statsKey, string(compressedPayload))
	require.NoError(t, err)
}

// verifyShardOwner checks that a shard has the expected owner and metadata
func verifyShardOwner(t *testing.T, cache *namespaceShardToExecutor, shardID, expectedExecutorID string, expectedMetadata map[string]string) {
	owner, err := cache.GetShardOwner(context.Background(), shardID)
	require.NoError(t, err)
	require.NotNil(t, owner)
	assert.Equal(t, expectedExecutorID, owner.ExecutorID)
	for key, expectedValue := range expectedMetadata {
		assert.Equal(t, expectedValue, owner.Metadata[key])
	}

	executor, err := cache.GetExecutor(context.Background(), expectedExecutorID)
	require.NoError(t, err)
	require.NotNil(t, executor)
	assert.Equal(t, expectedExecutorID, executor.ExecutorID)
	for key, expectedValue := range expectedMetadata {
		assert.Equal(t, expectedValue, executor.Metadata[key])
	}
}

type namespaceShardToExecutorTestCase struct {
	ctrl       *gomock.Controller
	e          *namespaceShardToExecutor
	etcdClient *etcdclient.MockClient
	timeSource clock.TimeSource

	watchChan chan clientv3.WatchResponse
	stopCh    chan struct{}

	executorID string
	prefix     string
	namespace  string

	executorPrefix string
}

func setupNamespaceShardToExecutorTestCase(t *testing.T) *namespaceShardToExecutorTestCase {
	var tc = new(namespaceShardToExecutorTestCase)

	tc.ctrl = gomock.NewController(t)
	logger := testlogger.New(t)

	tc.etcdClient = etcdclient.NewMockClient(tc.ctrl)
	tc.stopCh = make(chan struct{})
	tc.prefix = "/test-prefix"
	tc.namespace = "test-namespace"
	tc.executorID = "executor-1"
	tc.executorPrefix = etcdkeys.BuildExecutorsPrefix(tc.prefix, tc.namespace)

	// Mock the Watch call to return our watch channel
	tc.watchChan = make(chan clientv3.WatchResponse)
	tc.etcdClient.EXPECT().
		Watch(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(tc.watchChan).
		AnyTimes()

	e, err := newNamespaceShardToExecutor(tc.prefix, tc.namespace, tc.etcdClient, tc.stopCh, logger, clock.NewRealTimeSource(), metrics.NewNoopMetricsClient())
	require.NoError(t, err)
	tc.e = e
	return tc
}

// N concurrent cache-miss GetShardOwner calls must collapse into 1 etcd Get.
func TestNamespaceShardToExecutor_GetShardOwner_SingleFlightDedup(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer tc.ctrl.Finish()
	defer close(tc.stopCh)

	const numCallers = 50

	executorAssignedStateKey := etcdkeys.BuildExecutorKey(tc.prefix, tc.namespace, tc.executorID, etcdkeys.ExecutorAssignedStateKey)
	assignedState := &etcdtypes.AssignedState{
		AssignedShards: map[string]*types.ShardAssignment{
			"shard-1": {Status: types.AssignmentStatusREADY},
		},
	}
	assignedStateJSON, err := json.Marshal(assignedState)
	require.NoError(t, err)

	// release gates the Get so all callers pile up in singleflight first.
	release := make(chan struct{})
	var getCalls atomic.Int32
	tc.etcdClient.EXPECT().
		Get(gomock.Any(), tc.executorPrefix, gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, _ ...clientv3.OpOption) (*clientv3.GetResponse, error) {
			getCalls.Add(1)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return &clientv3.GetResponse{Header: &etcdserverpb.ResponseHeader{Revision: 1}, Kvs: []*mvccpb.KeyValue{
				{Key: []byte(executorAssignedStateKey), Value: assignedStateJSON},
			}}, nil
		}).
		// AnyTimes so a regression surfaces as a count mismatch, not "unexpected call".
		AnyTimes()

	var wg sync.WaitGroup
	owners := make([]*store.ShardOwner, numCallers)
	errs := make([]error, numCallers)
	start := make(chan struct{})
	for i := 0; i < numCallers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			owners[i], errs[i] = tc.e.GetShardOwner(context.Background(), "shard-1")
		}(i)
	}
	close(start)

	// Wait until the Get is in flight, then let other callers join the same singleflight.
	require.Eventually(t, func() bool { return getCalls.Load() >= 1 }, time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	close(release)
	wg.Wait()

	require.EqualValues(t, 1, getCalls.Load(), "concurrent misses should collapse to one etcd Get")
	for i := 0; i < numCallers; i++ {
		require.NoError(t, errs[i])
		require.NotNil(t, owners[i])
		assert.Equal(t, tc.executorID, owners[i].ExecutorID)
	}
}

// Cancelling the singleflight leader must not surface context.Canceled to other waiters.
func TestNamespaceShardToExecutor_GetShardOwner_CallerCancelDoesNotPoisonFlight(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer tc.ctrl.Finish()
	defer close(tc.stopCh)

	executorAssignedStateKey := etcdkeys.BuildExecutorKey(tc.prefix, tc.namespace, tc.executorID, etcdkeys.ExecutorAssignedStateKey)
	assignedState := &etcdtypes.AssignedState{
		AssignedShards: map[string]*types.ShardAssignment{
			"shard-1": {Status: types.AssignmentStatusREADY},
		},
	}
	assignedStateJSON, err := json.Marshal(assignedState)
	require.NoError(t, err)

	release := make(chan struct{})
	var getCalls atomic.Int32
	tc.etcdClient.EXPECT().
		Get(gomock.Any(), tc.executorPrefix, gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, _ ...clientv3.OpOption) (*clientv3.GetResponse, error) {
			getCalls.Add(1)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return &clientv3.GetResponse{Header: &etcdserverpb.ResponseHeader{Revision: 1}, Kvs: []*mvccpb.KeyValue{
				{Key: []byte(executorAssignedStateKey), Value: assignedStateJSON},
			}}, nil
		}).
		AnyTimes()

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancelledErrCh := make(chan error, 1)
	go func() {
		_, gerr := tc.e.GetShardOwner(cancelCtx, "shard-1")
		cancelledErrCh <- gerr
	}()

	survivorErrCh := make(chan error, 1)
	survivorOwnerCh := make(chan *store.ShardOwner, 1)
	go func() {
		o, gerr := tc.e.GetShardOwner(context.Background(), "shard-1")
		survivorErrCh <- gerr
		survivorOwnerCh <- o
	}()

	require.Eventually(t, func() bool { return getCalls.Load() >= 1 }, time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	cancel()

	select {
	case gerr := <-cancelledErrCh:
		require.Error(t, gerr)
		assert.ErrorIs(t, gerr, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("cancelled caller did not return after its context was cancelled")
	}

	close(release)
	select {
	case gerr := <-survivorErrCh:
		require.NoError(t, gerr)
	case <-time.After(time.Second):
		t.Fatal("survivor caller did not return after refresh completed")
	}
	owner := <-survivorOwnerCh
	require.NotNil(t, owner)
	assert.Equal(t, tc.executorID, owner.ExecutorID)
	assert.EqualValues(t, 1, getCalls.Load(), "the in-flight Get must outlive the cancelled leader")
}

// Concurrent stats misses for the same executor must collapse to 1 etcd Get.
func TestNamespaceShardToExecutor_GetExecutorStatistics_SingleFlightDedup(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer tc.ctrl.Finish()
	defer close(tc.stopCh)

	statsKey := etcdkeys.BuildExecutorKey(tc.prefix, tc.namespace, tc.executorID, etcdkeys.ExecutorShardStatisticsKey)

	stats := map[string]etcdtypes.ShardStatistics{
		"shard-1": {SmoothedLoad: 42.0},
	}
	payload, err := json.Marshal(stats)
	require.NoError(t, err)
	writer, err := common.NewRecordWriter("")
	require.NoError(t, err)
	compressedPayload, err := writer.Write(payload)
	require.NoError(t, err)

	release := make(chan struct{})
	var getCalls atomic.Int32
	tc.etcdClient.EXPECT().
		Get(gomock.Any(), statsKey).
		DoAndReturn(func(ctx context.Context, _ string, _ ...clientv3.OpOption) (*clientv3.GetResponse, error) {
			getCalls.Add(1)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return &clientv3.GetResponse{Kvs: []*mvccpb.KeyValue{
				{Key: []byte(statsKey), Value: compressedPayload},
			}}, nil
		}).
		AnyTimes()

	const numCallers = 20
	var wg sync.WaitGroup
	results := make([]map[string]etcdtypes.ShardStatistics, numCallers)
	errs := make([]error, numCallers)
	start := make(chan struct{})
	for i := 0; i < numCallers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = tc.e.GetExecutorStatistics(context.Background(), tc.executorID)
		}(i)
	}
	close(start)

	require.Eventually(t, func() bool { return getCalls.Load() >= 1 }, time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	close(release)
	wg.Wait()

	require.EqualValues(t, 1, getCalls.Load(), "concurrent stats misses should collapse to one etcd Get")
	for i := 0; i < numCallers; i++ {
		require.NoError(t, errs[i])
		assert.Equal(t, stats, results[i])
	}
}

// A hung etcd Get must not hold the singleflight key forever: the bounded
// refresh context must time out, surface DeadlineExceeded to waiters, and
// release the flight so the next caller can trigger a fresh Get.
func TestNamespaceShardToExecutor_GetShardOwner_RefreshContextHasBoundedTimeout(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer tc.ctrl.Finish()
	defer close(tc.stopCh)
	tc.e.refreshTimeout = 50 * time.Millisecond

	var getCalls atomic.Int32
	tc.etcdClient.EXPECT().
		Get(gomock.Any(), tc.executorPrefix, gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, _ ...clientv3.OpOption) (*clientv3.GetResponse, error) {
			getCalls.Add(1)
			<-ctx.Done()
			return nil, ctx.Err()
		}).
		AnyTimes()

	// Use a generous caller deadline so the timeout we observe must come from
	// the refresh's own bounded context, not the caller's.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := tc.e.GetShardOwner(ctx, "shard-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.EqualValues(t, 1, getCalls.Load())

	// Flight must be released so the next caller triggers a fresh Get.
	_, err = tc.e.GetShardOwner(ctx, "shard-1")
	require.Error(t, err)
	assert.EqualValues(t, 2, getCalls.Load(), "second caller should trigger a new Get, not join the previous flight")
}
