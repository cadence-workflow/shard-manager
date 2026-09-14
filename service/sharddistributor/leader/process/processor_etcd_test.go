package process

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/dynamicconfig/dynamicproperties"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdclient"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/executorstore"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/testhelper"
)

// TestRebalanceShards_DrainedHostUnassignsThroughEtcd shows that DrainHosts
// only records the drain: shards stay assigned, and GetShardOwner still
// returns the drained-host executor. The leader rebalance loop is what
// empties that assignment and moves the shards.
func TestRebalanceShards_DrainedHostUnassignsThroughEtcd(t *testing.T) {
	const (
		drainedHostExecutor = "host-a@uuid-1"
		healthyHostExecutor = "host-b@uuid-1"
	)
	tc := testhelper.SetupStoreTestCluster(t)
	timeSource := clock.NewMockedTimeSourceAt(time.Now())
	executorStore := newEtcdStore(t, tc, timeSource)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, drainedHostExecutor, store.HeartbeatState{
		Status:        types.ExecutorStatusACTIVE,
		LastHeartbeat: timeSource.Now(),
	}))
	require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, healthyHostExecutor, store.HeartbeatState{
		Status:        types.ExecutorStatusACTIVE,
		LastHeartbeat: timeSource.Now(),
	}))
	assignShardsForTest(ctx, t, executorStore, tc.Namespace, drainedHostExecutor, "0", "1")

	require.NoError(t, executorStore.DrainHosts(ctx, tc.Namespace, []store.DrainedHost{
		{Hostname: "host-a"},
	}))

	state, err := executorStore.GetState(ctx, tc.Namespace)
	require.NoError(t, err)
	assert.False(t, state.IsExecutorAssignable(drainedHostExecutor, nil))
	assert.True(t, state.IsExecutorAssignable(healthyHostExecutor, nil))
	assert.Len(t, state.ShardAssignments[drainedHostExecutor].AssignedShards, 2, "DrainHosts must not itself unassign shards")

	require.Eventually(t, func() bool {
		owner, err := executorStore.GetShardOwner(ctx, tc.Namespace, "0")
		return err == nil && owner.ExecutorID == drainedHostExecutor
	}, 5*time.Second, 50*time.Millisecond, "read path keeps the drained-host owner until rebalance")

	processor := newEtcdProcessor(t, tc.Namespace, executorStore, timeSource)
	require.NoError(t, processor.rebalanceShards(ctx))

	state, err = executorStore.GetState(ctx, tc.Namespace)
	require.NoError(t, err)
	assert.Empty(t, state.ShardAssignments[drainedHostExecutor].AssignedShards)
	assert.Len(t, state.ShardAssignments[healthyHostExecutor].AssignedShards, 2)

	require.Eventually(t, func() bool {
		owner, err := executorStore.GetShardOwner(ctx, tc.Namespace, "0")
		return err == nil && owner.ExecutorID == healthyHostExecutor
	}, 5*time.Second, 50*time.Millisecond)
}

func newEtcdStore(t *testing.T, tc *testhelper.StoreTestCluster, timeSource clock.TimeSource) store.Store {
	t.Helper()

	etcdConfig, err := etcdclient.NewExecutorStoreConfig(tc.SDConfig)
	require.NoError(t, err)

	lc := fxtest.NewLifecycle(t)
	s, err := executorstore.NewStore(executorstore.ExecutorStoreParams{
		Client:        tc.Client,
		ETCDConfig:    etcdConfig,
		Lifecycle:     lc,
		Logger:        testlogger.New(t),
		TimeSource:    timeSource,
		MetricsClient: metrics.NewNoopMetricsClient(),
		Config: &config.Config{
			LoadBalancingMode: func(namespace string) string { return config.LoadBalancingModeNAIVE },
			MaxEtcdTxnOps:     dynamicproperties.GetIntPropertyFn(128),
			LoadBalancingNaive: config.LoadBalancingNaiveConfig{
				MaxDeviation: func(namespace string) float64 { return 2.0 },
			},
		},
	})
	require.NoError(t, err)
	lc.RequireStart()
	t.Cleanup(lc.RequireStop)
	return s
}

func newEtcdProcessor(t *testing.T, namespace string, executorStore store.Store, timeSource clock.TimeSource) *namespaceProcessor {
	t.Helper()

	ctrl := gomock.NewController(t)
	election := store.NewMockElection(ctrl)
	election.EXPECT().Guard().Return(store.NopGuard()).AnyTimes()

	factory := NewProcessorFactory(
		testlogger.New(t),
		metrics.NewNoopMetricsClient(),
		timeSource,
		config.ShardDistribution{
			Process: config.LeaderProcess{
				Period:       time.Second,
				HeartbeatTTL: time.Minute,
			},
		},
		&config.Config{
			LoadBalancingMode: func(namespace string) string { return config.LoadBalancingModeNAIVE },
			LoadBalancingNaive: config.LoadBalancingNaiveConfig{
				MaxDeviation: func(namespace string) float64 { return 2.0 },
			},
		},
	)
	return factory.CreateProcessor(config.Namespace{
		Name:     namespace,
		ShardNum: 2,
		Type:     config.NamespaceTypeFixed,
	}, executorStore, election).(*namespaceProcessor)
}

func assignShardsForTest(ctx context.Context, t *testing.T, executorStore store.Store, namespace, executorID string, shardIDs ...string) {
	t.Helper()

	namespaceState, err := executorStore.GetState(ctx, namespace)
	require.NoError(t, err)

	assignedState := namespaceState.ShardAssignments[executorID]
	if assignedState.AssignedShards == nil {
		assignedState.AssignedShards = make(map[string]*types.ShardAssignment)
	}
	for _, shardID := range shardIDs {
		assignedState.AssignedShards[shardID] = &types.ShardAssignment{Status: types.AssignmentStatusREADY}
	}
	namespaceState.ShardAssignments[executorID] = assignedState

	require.NoError(t, executorStore.AssignShards(ctx, namespace, store.AssignShardsRequest{NewState: namespaceState}, store.NopGuard()))
}
