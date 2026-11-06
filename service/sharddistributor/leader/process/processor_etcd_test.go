package process

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/dynamicconfig/dynamicproperties"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdclient"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/executorstore"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/testhelper"
)

// The client is consumed by name, so it has to be provided as one.
type namedExecutorStoreClient struct {
	fx.Out

	Client etcdclient.Client `name:"executorstore"`
}

// TestRebalanceShards_DrainedHostUnassignsThroughEtcd shows that DrainHosts
// only records the drain: shards stay assigned to the drained-host executor.
// The leader rebalance loop is what empties that assignment and moves the shards.
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
		HostMetadata:  &types.HostMetadata{HostName: "host-a"},
	}))
	require.NoError(t, executorStore.RecordHeartbeat(ctx, tc.Namespace, healthyHostExecutor, store.HeartbeatState{
		Status:        types.ExecutorStatusACTIVE,
		LastHeartbeat: timeSource.Now(),
		HostMetadata:  &types.HostMetadata{HostName: "host-b"},
	}))
	assignShardsForTest(ctx, t, executorStore, tc.Namespace, drainedHostExecutor, "0", "1")

	require.NoError(t, executorStore.DrainHosts(ctx, tc.Namespace, []store.DrainedHost{
		{Hostname: "host-a"},
	}))

	state, err := executorStore.GetState(ctx, tc.Namespace)
	require.NoError(t, err)

	drained, ok := state.Executor(drainedHostExecutor)
	require.True(t, ok)
	assert.False(t, drained.IsAssignable(nil))

	healthy, ok := state.Executor(healthyHostExecutor)
	require.True(t, ok)
	assert.True(t, healthy.IsAssignable(nil))

	assert.Len(t, state.ShardAssignments[drainedHostExecutor].AssignedShards, 2, "DrainHosts must not itself unassign shards")
	assert.Equal(t, drainedHostExecutor, state.ShardOwners()["0"], "assignment keeps the drained-host owner until rebalance")

	processor := newEtcdProcessor(t, tc.Namespace, executorStore, timeSource)
	require.NoError(t, processor.rebalanceShards(ctx))

	state, err = executorStore.GetState(ctx, tc.Namespace)
	require.NoError(t, err)
	assert.Empty(t, state.ShardAssignments[drainedHostExecutor].AssignedShards)
	assert.Len(t, state.ShardAssignments[healthyHostExecutor].AssignedShards, 2)
	assert.Equal(t, healthyHostExecutor, state.ShardOwners()["0"])
}

func newEtcdStore(t *testing.T, tc *testhelper.StoreTestCluster, timeSource clock.TimeSource) store.Store {
	t.Helper()

	etcdConfig, err := etcdclient.NewExecutorStoreConfig(tc.SDConfig)
	require.NoError(t, err)

	var s store.Store
	app := fxtest.New(t,
		executorstore.Module,
		fx.Provide(func() namedExecutorStoreClient {
			return namedExecutorStoreClient{Client: tc.Client}
		}),
		fx.Provide(func() etcdclient.ExecutorStoreConfig { return etcdConfig }),
		fx.Provide(func() log.Logger { return testlogger.New(t) }),
		fx.Provide(func() clock.TimeSource { return timeSource }),
		fx.Provide(func() metrics.Client { return metrics.NewNoopMetricsClient() }),
		fx.Provide(func() *config.Config {
			return &config.Config{
				LoadBalancingMode: func(namespace string) string { return config.LoadBalancingModeNAIVE },
				MaxEtcdTxnOps:     dynamicproperties.GetIntPropertyFn(128),
				LoadBalancingNaive: config.LoadBalancingNaiveConfig{
					MaxDeviation: func(namespace string) float64 { return 2.0 },
				},
			}
		}),
		fx.Populate(&s),
	)
	app.RequireStart()
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
