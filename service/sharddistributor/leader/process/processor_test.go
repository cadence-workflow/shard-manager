package process

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/cadence-workflow/shard-manager/common"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	metricmocks "github.com/cadence-workflow/shard-manager/common/metrics/mocks"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/loadbalancer/plan"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

type testDependencies struct {
	ctrl         *gomock.Controller
	store        *store.MockStore
	election     *store.MockElection
	timeSource   clock.MockedTimeSource
	factory      Factory
	cfg          config.Namespace
	sdConfig     *config.Config
	observedLogs *observer.ObservedLogs
}

func setupProcessorTest(t *testing.T, namespaceType string) *testDependencies {
	ctrl := gomock.NewController(t)
	mockedClock := clock.NewMockedTimeSource()
	deps := &testDependencies{
		ctrl:       ctrl,
		store:      store.NewMockStore(ctrl),
		election:   store.NewMockElection(ctrl),
		timeSource: mockedClock,
		cfg:        config.Namespace{Name: "test-ns", ShardNum: 2, Type: namespaceType},
	}
	deps.sdConfig = &config.Config{
		LoadBalancingMode: func(namespace string) string {
			return config.LoadBalancingModeNAIVE
		},
		LoadBalancingNaive: config.LoadBalancingNaiveConfig{
			MaxDeviation: func(namespace string) float64 {
				return 2.0
			},
		},
	}

	logger, observedLogs := testlogger.NewObserved(t)
	deps.observedLogs = observedLogs

	deps.factory = NewProcessorFactory(
		logger,
		metrics.NewNoopMetricsClient(),
		mockedClock,
		config.ShardDistribution{
			Process: config.LeaderProcess{
				Period:       time.Second,
				HeartbeatTTL: time.Second,
			},
		},
		deps.sdConfig,
	)
	return deps
}

func TestRunAndTerminate(t *testing.T) {
	defer goleak.VerifyNone(t)

	mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
	defer mocks.ctrl.Finish()
	processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election)
	ctx, cancel := context.WithCancel(context.Background())

	mocks.store.EXPECT().GetState(gomock.Any(), mocks.cfg.Name).Return(&store.NamespaceState{}, nil).AnyTimes()
	mocks.store.EXPECT().SubscribeToExecutorStatusChanges(gomock.Any(), mocks.cfg.Name).Return(make(chan int64), nil).AnyTimes()

	err := processor.Run(ctx)
	require.NoError(t, err)

	err = processor.Run(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "processor is already running")

	err = processor.Terminate(context.Background())
	require.NoError(t, err)

	err = processor.Terminate(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "processor has not been started")

	cancel()
}

func TestRebalanceShards_InitialDistribution(t *testing.T) {
	mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
	defer mocks.ctrl.Finish()
	processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

	now := mocks.timeSource.Now()
	state := map[string]store.HeartbeatState{
		"exec-1": {Status: types.ExecutorStatusACTIVE, LastHeartbeat: now},
		"exec-2": {Status: types.ExecutorStatusACTIVE, LastHeartbeat: now},
	}
	mocks.store.EXPECT().GetState(gomock.Any(), mocks.cfg.Name).Return(&store.NamespaceState{Executors: state}, nil)
	mocks.election.EXPECT().Guard().Return(store.NopGuard())
	mocks.store.EXPECT().AssignShards(gomock.Any(), mocks.cfg.Name, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, request store.AssignShardsRequest, _ store.GuardFunc) error {
			assert.Len(t, request.NewState.ShardAssignments, 2)
			assert.Len(t, request.NewState.ShardAssignments["exec-1"].AssignedShards, 1)
			assert.Len(t, request.NewState.ShardAssignments["exec-2"].AssignedShards, 1)
			assert.Lenf(t, request.NewState.ShardAssignments["exec-1"].ShardHandoverStats, 0, "no handover stats should be present on initial assignment")
			assert.Lenf(t, request.NewState.ShardAssignments["exec-2"].ShardHandoverStats, 0, "no handover stats should be present on initial assignment")
			return nil
		},
	)

	err := processor.rebalanceShards(context.Background())
	require.NoError(t, err)
}

func TestRebalanceShards_ExecutorRemoved(t *testing.T) {
	mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
	defer mocks.ctrl.Finish()
	processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

	now := mocks.timeSource.Now()
	heartbeats := map[string]store.HeartbeatState{
		"exec-1": {Status: types.ExecutorStatusACTIVE, LastHeartbeat: now},
		"exec-2": {Status: types.ExecutorStatusDRAINING, LastHeartbeat: now},
	}
	assignments := map[string]store.AssignedState{
		"exec-2": {
			AssignedShards: map[string]*types.ShardAssignment{
				"0": {Status: types.AssignmentStatusREADY},
				"1": {Status: types.AssignmentStatusREADY},
			},
		},
	}
	mocks.store.EXPECT().GetState(gomock.Any(), mocks.cfg.Name).Return(&store.NamespaceState{
		Executors:        heartbeats,
		ShardAssignments: assignments,
	}, nil)
	mocks.election.EXPECT().Guard().Return(store.NopGuard())
	mocks.store.EXPECT().AssignShards(gomock.Any(), mocks.cfg.Name, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, request store.AssignShardsRequest, _ store.GuardFunc) error {
			assert.Len(t, request.NewState.ShardAssignments["exec-1"].AssignedShards, 2)
			assert.Len(t, request.NewState.ShardAssignments["exec-2"].AssignedShards, 0)
			assert.Lenf(t, request.NewState.ShardAssignments["exec-1"].ShardHandoverStats, 2, "both shards move from the draining executor in the snapshot")
			assert.Lenf(t, request.NewState.ShardAssignments["exec-2"].ShardHandoverStats, 0, "no handover stats should be present for drained executor")
			return nil
		},
	)

	err := processor.rebalanceShards(context.Background())
	require.NoError(t, err)
}

func TestRebalanceShards_ClearsAssignmentOfLiveDrainingExecutor(t *testing.T) {
	mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
	defer mocks.ctrl.Finish()
	processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

	now := mocks.timeSource.Now()
	mocks.store.EXPECT().GetState(gomock.Any(), mocks.cfg.Name).Return(&store.NamespaceState{
		Executors: map[string]store.HeartbeatState{
			"exec-active":   {Status: types.ExecutorStatusACTIVE, LastHeartbeat: now},
			"exec-draining": {Status: types.ExecutorStatusDRAINING, LastHeartbeat: now},
		},
		ShardAssignments: map[string]store.AssignedState{
			"exec-draining": {
				AssignedShards: map[string]*types.ShardAssignment{
					"0": {Status: types.AssignmentStatusREADY},
					"1": {Status: types.AssignmentStatusREADY},
				},
				ModRevision: 7,
			},
		},
	}, nil)
	mocks.election.EXPECT().Guard().Return(store.NopGuard())
	mocks.store.EXPECT().AssignShards(gomock.Any(), mocks.cfg.Name, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, request store.AssignShardsRequest, _ store.GuardFunc) error {
			assert.Len(t, request.NewState.ShardAssignments["exec-active"].AssignedShards, 2)

			drainingAssignment := request.NewState.ShardAssignments["exec-draining"]
			assert.Empty(t, drainingAssignment.AssignedShards, "the draining executor must stop being told to serve its old shards")
			assert.Equal(t, int64(7), drainingAssignment.ModRevision)
			assert.Contains(t, request.ChangedExecutors, "exec-draining", "the emptied assignment has to be written out")
			assert.Empty(t, request.ExecutorsToDelete, "a heartbeating executor is not stale")
			return nil
		},
	)

	err := processor.rebalanceShards(context.Background())
	require.NoError(t, err)
}

func TestRebalanceShards_ExecutorStale(t *testing.T) {
	mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
	defer mocks.ctrl.Finish()
	processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

	now := mocks.timeSource.Now()
	heartbeats := map[string]store.HeartbeatState{
		"exec-1": {Status: types.ExecutorStatusACTIVE, LastHeartbeat: now},
		"exec-2": {Status: types.ExecutorStatusACTIVE, LastHeartbeat: now.Add(-2 * time.Second)},
	}
	assignments := map[string]store.AssignedState{
		"exec-1": {
			AssignedShards: map[string]*types.ShardAssignment{
				"0": {Status: types.AssignmentStatusREADY},
			},
			ModRevision: 1,
		},
		"exec-2": {
			AssignedShards: map[string]*types.ShardAssignment{
				"1": {Status: types.AssignmentStatusREADY},
			},
			ModRevision: 1,
		},
	}
	mocks.store.EXPECT().GetState(gomock.Any(), mocks.cfg.Name).Return(&store.NamespaceState{
		Executors:        heartbeats,
		ShardAssignments: assignments,
	}, nil)
	mocks.election.EXPECT().Guard().Return(store.NopGuard())
	mocks.store.EXPECT().AssignShards(gomock.Any(), mocks.cfg.Name, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, request store.AssignShardsRequest, _ store.GuardFunc) error {
			assert.Len(t, request.NewState.ShardAssignments, 1)
			assert.Len(t, request.NewState.ShardAssignments["exec-1"].AssignedShards, 2)
			assert.Len(t, request.NewState.ShardAssignments["exec-1"].ShardHandoverStats, 1, "only shard 1 should have handover stats")
			assert.Equal(t, request.ExecutorsToDelete, map[string]int64{"exec-2": 1})
			return nil
		},
	)

	err := processor.rebalanceShards(context.Background())
	require.NoError(t, err)
}

func TestRebalanceShards_NoActiveExecutors(t *testing.T) {
	mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
	defer mocks.ctrl.Finish()
	processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

	now := mocks.timeSource.Now()
	state := map[string]store.HeartbeatState{
		"exec-1": {Status: types.ExecutorStatusDRAINING, LastHeartbeat: now},
	}
	mocks.store.EXPECT().GetState(gomock.Any(), mocks.cfg.Name).Return(&store.NamespaceState{Executors: state}, nil)

	err := processor.rebalanceShards(context.Background())
	require.NoError(t, err)
}

func TestRebalanceShards_NoActiveExecutors_WithStaleExecutors(t *testing.T) {
	t.Run("one stale executor", func(t *testing.T) {
		mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
		defer mocks.ctrl.Finish()
		processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

		now := mocks.timeSource.Now()
		executorStates := map[string]store.HeartbeatState{
			"exec-1": {Status: types.ExecutorStatusDRAINING, LastHeartbeat: now},
			"exec-2": {Status: types.ExecutorStatusACTIVE, LastHeartbeat: now.Add(-10 * time.Minute)},
		}
		expectedStaleExecutorIDs := []string{"exec-2"}

		mocks.store.EXPECT().GetState(gomock.Any(), mocks.cfg.Name).Return(&store.NamespaceState{
			Executors: executorStates,
		}, nil)

		mocks.election.EXPECT().Guard().Return(store.NopGuard())
		mocks.store.EXPECT().DeleteExecutors(gomock.Any(), mocks.cfg.Name, gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, namespace string, executorIDs []string, _ store.GuardFunc) error {
				assert.ElementsMatch(t, expectedStaleExecutorIDs, executorIDs)
				assert.Equal(t, mocks.cfg.Name, namespace)
				return nil
			})

		err := processor.rebalanceShards(context.Background())
		require.NoError(t, err)
	})

	t.Run("all stale executor", func(t *testing.T) {
		mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
		defer mocks.ctrl.Finish()
		processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

		now := mocks.timeSource.Now()
		executorStates := map[string]store.HeartbeatState{
			"exec-1": {Status: types.ExecutorStatusACTIVE, LastHeartbeat: now.Add(-10 * time.Minute)},
			"exec-2": {Status: types.ExecutorStatusACTIVE, LastHeartbeat: now.Add(-10 * time.Minute)},
		}
		expectedStaleExecutorIDs := []string{"exec-1", "exec-2"}

		mocks.store.EXPECT().GetState(gomock.Any(), mocks.cfg.Name).Return(&store.NamespaceState{
			Executors: executorStates,
		}, nil)

		mocks.election.EXPECT().Guard().Return(store.NopGuard())
		mocks.store.EXPECT().DeleteExecutors(gomock.Any(), mocks.cfg.Name, gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, namespace string, executorIDs []string, _ store.GuardFunc) error {
				assert.ElementsMatch(t, expectedStaleExecutorIDs, executorIDs)
				assert.Equal(t, mocks.cfg.Name, namespace)
				return nil
			})

		err := processor.rebalanceShards(context.Background())
		require.NoError(t, err)
	})
}

func TestCleanupStaleExecutors(t *testing.T) {
	mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
	defer mocks.ctrl.Finish()
	processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)
	now := mocks.timeSource.Now()

	heartbeats := map[string]store.HeartbeatState{
		"exec-active": {LastHeartbeat: now},
		"exec-stale":  {LastHeartbeat: now.Add(-_defaultHeartbeatTTL).Add(-1 * time.Second)},
		"exec-orphan": {},
	}

	namespaceState := &store.NamespaceState{
		Executors: heartbeats,
		ShardAssignments: map[string]store.AssignedState{
			"exec-orphan": {ModRevision: 12},
		},
	}

	staleExecutors := processor.identifyStaleExecutors(namespaceState)
	assert.Equal(t, map[string]int64{"exec-stale": 0, "exec-orphan": 12}, staleExecutors)
}

func TestCleanupStaleShardStats(t *testing.T) {
	t.Run("stale shard stats are deleted", func(t *testing.T) {
		mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
		defer mocks.ctrl.Finish()
		processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

		now := mocks.timeSource.Now().UTC()

		heartbeats := map[string]store.HeartbeatState{
			"exec-active": {LastHeartbeat: now, Status: types.ExecutorStatusACTIVE},
			"exec-stale":  {LastHeartbeat: now.Add(-_defaultHeartbeatTTL).Add(-1 * time.Second)},
		}

		assignments := map[string]store.AssignedState{
			"exec-active": {
				AssignedShards: map[string]*types.ShardAssignment{
					"shard-1": {Status: types.AssignmentStatusREADY},
					"shard-2": {Status: types.AssignmentStatusREADY},
				},
			},
			"exec-stale": {
				AssignedShards: map[string]*types.ShardAssignment{
					"shard-3": {Status: types.AssignmentStatusREADY},
				},
			},
		}

		staleCutoff := now.Add(-_defaultHeartbeatTTL).Add(-1 * time.Second)
		shardStats := map[string]store.ShardStatistics{
			"shard-1": {SmoothedLoad: 1.0, LastUpdateTime: now, LastMoveTime: now},
			"shard-2": {SmoothedLoad: 2.0, LastUpdateTime: now, LastMoveTime: now},
			"shard-3": {SmoothedLoad: 3.0, LastUpdateTime: staleCutoff, LastMoveTime: staleCutoff},
		}

		namespaceState := &store.NamespaceState{
			Executors:        heartbeats,
			ShardAssignments: assignments,
			ShardStats:       shardStats,
		}

		staleShardStats := processor.identifyStaleShardStats(namespaceState)
		assert.Equal(t, []string{"shard-3"}, staleShardStats)
	})

	t.Run("recent shard stats are preserved", func(t *testing.T) {
		mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
		defer mocks.ctrl.Finish()
		processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

		now := mocks.timeSource.Now()

		expiredExecutor := now.Add(-_defaultHeartbeatTTL).Add(-1 * time.Second)
		namespaceState := &store.NamespaceState{
			Executors: map[string]store.HeartbeatState{
				"exec-stale": {LastHeartbeat: expiredExecutor},
			},
			ShardAssignments: map[string]store.AssignedState{},
			ShardStats: map[string]store.ShardStatistics{
				"shard-1": {SmoothedLoad: 5.0, LastUpdateTime: now, LastMoveTime: now},
			},
		}

		staleShardStats := processor.identifyStaleShardStats(namespaceState)
		assert.Empty(t, staleShardStats)
	})

}

func TestRebalance_StoreErrors(t *testing.T) {
	mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
	defer mocks.ctrl.Finish()
	processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)
	expectedErr := errors.New("store is down")

	mocks.store.EXPECT().GetState(gomock.Any(), mocks.cfg.Name).Return(nil, expectedErr)
	err := processor.rebalanceShards(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), expectedErr.Error())

	now := mocks.timeSource.Now()
	mocks.store.EXPECT().GetState(gomock.Any(), mocks.cfg.Name).Return(&store.NamespaceState{
		Executors: map[string]store.HeartbeatState{"e": {Status: types.ExecutorStatusACTIVE, LastHeartbeat: now}},
	}, nil)
	mocks.election.EXPECT().Guard().Return(store.NopGuard())
	mocks.store.EXPECT().AssignShards(gomock.Any(), mocks.cfg.Name, gomock.Any(), gomock.Any()).Return(expectedErr)
	err = processor.rebalanceShards(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), expectedErr.Error())
}

func TestRunLoop_SubscriptionError(t *testing.T) {
	mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
	defer mocks.ctrl.Finish()
	processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

	expectedErr := errors.New("subscription failed")
	mocks.store.EXPECT().GetState(gomock.Any(), mocks.cfg.Name).Return(&store.NamespaceState{}, nil)
	mocks.store.EXPECT().SubscribeToExecutorStatusChanges(gomock.Any(), mocks.cfg.Name).Return(nil, expectedErr)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		processor.runRebalancingLoop(context.Background())
	}()
	wg.Wait()
}

func TestRunLoop_ContextCancellation(t *testing.T) {
	mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
	defer mocks.ctrl.Finish()
	processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)
	ctx, cancel := context.WithCancel(context.Background())

	// Setup for the initial call to rebalanceShards and the subscription
	mocks.store.EXPECT().GetState(gomock.Any(), mocks.cfg.Name).Return(&store.NamespaceState{}, nil)
	mocks.store.EXPECT().SubscribeToExecutorStatusChanges(gomock.Any(), mocks.cfg.Name).Return(make(chan int64), nil)

	processor.wg.Add(1)
	// Run the process in a separate goroutine to avoid blocking the test
	go processor.runProcess(ctx)

	// Wait for the two loops (rebalance and cleanup) to create their tickers
	mocks.timeSource.BlockUntil(2)

	// Now, cancel the context to signal the loops to stop
	cancel()

	// Wait for the main process loop to exit gracefully
	processor.wg.Wait()
}

func TestRebalanceShards_NoShardsToReassign(t *testing.T) {
	mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
	defer mocks.ctrl.Finish()
	processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)
	testScope := tally.NewTestScope("test", nil)
	processor.metricsClient = metrics.NewClient(testScope, metrics.ShardDistributor, metrics.MigrationConfig{})

	now := mocks.timeSource.Now()
	heartbeats := map[string]store.HeartbeatState{
		// Set the last heartbeat to 500ms ago to verify the oldest executor heartbeat lag metric.
		"exec-1": {Status: types.ExecutorStatusACTIVE, LastHeartbeat: now.Add(-500 * time.Millisecond)},
	}
	assignments := map[string]store.AssignedState{
		"exec-1": {
			AssignedShards: map[string]*types.ShardAssignment{
				"0": {Status: types.AssignmentStatusREADY},
				"1": {Status: types.AssignmentStatusREADY},
			},
		},
	}
	mocks.store.EXPECT().GetState(gomock.Any(), mocks.cfg.Name).Return(&store.NamespaceState{
		Executors:        heartbeats,
		ShardAssignments: assignments,
	}, nil)

	err := processor.rebalanceShards(context.Background())
	require.NoError(t, err)

	gauges := testScope.Snapshot().Gauges()
	metricTags := "namespace=test-ns,namespace_type=fixed,operation=ShardAssignLoop"
	assert.Equal(t, float64(2), gauges["test.shard_distributor_active_shards+"+metricTags].Value())
	assert.Equal(t, float64(500), gauges["test.shard_distributor_oldest_executor_heartbeat_lag+"+metricTags].Value())
}

func TestFindDrainedAssignedShards(t *testing.T) {
	activeExecutors := []string{"exec-1", "exec-2"}

	tests := []struct {
		name          string
		state         *store.NamespaceState
		activeExec    []string
		drainedShards []string
	}{
		{
			name: "nothing drained",
			state: &store.NamespaceState{
				ShardAssignments: assignmentsFor("exec-1", "0", "1"),
				DrainedShards:    nil,
			},
			activeExec:    activeExecutors,
			drainedShards: nil,
		},
		{
			name: "drained shards assigned to an active executor",
			state: &store.NamespaceState{
				ShardAssignments: assignmentsFor("exec-1", "0", "1", "2", "3"),
				DrainedShards:    map[string]struct{}{"0": {}, "1": {}, "2": {}, "3": {}},
			},
			activeExec:    activeExecutors,
			drainedShards: []string{"0", "1", "2", "3"},
		},
		{
			name: "drained shard assigned to an inactive executor",
			state: &store.NamespaceState{
				ShardAssignments: assignmentsFor("exec-inactive", "0", "1"),
				DrainedShards:    map[string]struct{}{"1": {}},
			},
			activeExec:    activeExecutors,
			drainedShards: []string{},
		},
		{
			name: "drained shard is not assigned to any executor",
			state: &store.NamespaceState{
				ShardAssignments: assignmentsFor("exec-1", "0", "2"),
				DrainedShards:    map[string]struct{}{"1": {}},
			},
			activeExec:    activeExecutors,
			drainedShards: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ElementsMatch(t, tt.drainedShards, findDrainedAssignedShards(tt.state, tt.activeExec))
		})
	}
}

func assignmentsFor(executorID string, shardIDs ...string) map[string]store.AssignedState {
	assigned := make(map[string]*types.ShardAssignment, len(shardIDs))
	for _, shardID := range shardIDs {
		assigned[shardID] = &types.ShardAssignment{Status: types.AssignmentStatusREADY}
	}
	return map[string]store.AssignedState{executorID: {AssignedShards: assigned}}
}

func TestRebalanceShards_DrainedShardsAreDroppedFromExecutors(t *testing.T) {
	for _, namespaceType := range []string{config.NamespaceTypeFixed, config.NamespaceTypeEphemeral} {
		t.Run(namespaceType, func(t *testing.T) {
			mocks := setupProcessorTest(t, namespaceType)
			defer mocks.ctrl.Finish()

			processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

			now := mocks.timeSource.Now()
			mocks.store.EXPECT().GetState(gomock.Any(), mocks.cfg.Name).Return(&store.NamespaceState{
				Executors: map[string]store.HeartbeatState{
					"exec-1": {Status: types.ExecutorStatusACTIVE, LastHeartbeat: now},
				},
				ShardAssignments: assignmentsFor("exec-1", "0", "1"),
				DrainedShards:    map[string]struct{}{"1": {}},
			}, nil)

			// Shard "1" is drained, so only active shard "0" remains assigned.
			mocks.election.EXPECT().Guard().Return(store.NopGuard())

			var request store.AssignShardsRequest
			mocks.store.EXPECT().AssignShards(gomock.Any(), mocks.cfg.Name, gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, _ string, req store.AssignShardsRequest, _ store.GuardFunc) error {
					request = req
					return nil
				})

			// Triggering rebalance to reassign the shards
			require.NoError(t, processor.rebalanceShards(context.Background()))

			assigned := request.NewState.ShardAssignments["exec-1"].AssignedShards

			assert.Equal(t, []string{"0"}, slices.Sorted(maps.Keys(assigned)), "drained shard is dropped")
			assert.Contains(t, request.ChangedExecutors, "exec-1", "exec-1 had the drained shard")
		})
	}
}

func TestRebalanceShards_AlreadyUnassignedDrainedShardsSkipAssign(t *testing.T) {
	mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
	defer mocks.ctrl.Finish()

	processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

	now := mocks.timeSource.Now()

	// shard "2" is drained and unassigned
	mocks.store.EXPECT().GetState(gomock.Any(), mocks.cfg.Name).Return(&store.NamespaceState{
		Executors: map[string]store.HeartbeatState{
			"exec-1": {Status: types.ExecutorStatusACTIVE, LastHeartbeat: now},
		},
		ShardAssignments: assignmentsFor("exec-1", "0", "1"),
		DrainedShards:    map[string]struct{}{"2": {}},
	}, nil)

	// shards should not move, drained shards should not be assigned back
	require.NoError(t, processor.rebalanceShards(context.Background()))
	assert.NotEmpty(t, mocks.observedLogs.FilterMessage("No changes to distribution detected. Skipping rebalance.").All())

}

func TestRebalanceShards_WithUnassignedShards(t *testing.T) {
	mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
	defer mocks.ctrl.Finish()
	processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

	now := mocks.timeSource.Now()
	heartbeats := map[string]store.HeartbeatState{
		"exec-1": {Status: types.ExecutorStatusACTIVE, LastHeartbeat: now},
	}
	// Note: shard "1" is missing from assignments
	assignments := map[string]store.AssignedState{
		"exec-1": {
			AssignedShards: map[string]*types.ShardAssignment{
				"0": {Status: types.AssignmentStatusREADY},
			},
		},
	}
	mocks.store.EXPECT().GetState(gomock.Any(), mocks.cfg.Name).Return(&store.NamespaceState{
		Executors:        heartbeats,
		ShardAssignments: assignments,
	}, nil)
	mocks.election.EXPECT().Guard().Return(store.NopGuard())
	mocks.store.EXPECT().AssignShards(gomock.Any(), mocks.cfg.Name, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, request store.AssignShardsRequest, _ store.GuardFunc) error {
			assert.Len(t, request.NewState.ShardAssignments["exec-1"].AssignedShards, 2, "Both shards should now be assigned to exec-1")
			return nil
		},
	)

	err := processor.rebalanceShards(context.Background())
	require.NoError(t, err)
}

func TestRebalanceShards_AppliesNaiveLoadBalancingPlan(t *testing.T) {
	mocks := setupProcessorTest(t, config.NamespaceTypeEphemeral)
	defer mocks.ctrl.Finish()
	processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

	now := mocks.timeSource.Now()
	// exec-2 is overloaded, so naive should move one shard to exec-1.
	heartbeats := map[string]store.HeartbeatState{
		"exec-1": {
			Status:        types.ExecutorStatusACTIVE,
			LastHeartbeat: now,
			ReportedShards: map[string]*types.ShardStatusReport{
				"shard-1": {ShardLoad: 5.0},
			},
		},
		"exec-2": {
			Status:        types.ExecutorStatusACTIVE,
			LastHeartbeat: now,
			ReportedShards: map[string]*types.ShardStatusReport{
				"shard-2": {ShardLoad: 30.0},
				"shard-3": {ShardLoad: 20.0},
			},
		},
	}
	assignments := map[string]store.AssignedState{
		"exec-1": {AssignedShards: map[string]*types.ShardAssignment{"shard-1": {Status: types.AssignmentStatusREADY}}},
		"exec-2": {AssignedShards: map[string]*types.ShardAssignment{"shard-2": {Status: types.AssignmentStatusREADY}, "shard-3": {Status: types.AssignmentStatusREADY}}},
	}

	mocks.store.EXPECT().GetState(gomock.Any(), mocks.cfg.Name).Return(&store.NamespaceState{
		Executors:        heartbeats,
		ShardAssignments: assignments,
	}, nil)
	mocks.election.EXPECT().Guard().Return(store.NopGuard())
	mocks.store.EXPECT().AssignShards(gomock.Any(), mocks.cfg.Name, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, request store.AssignShardsRequest, _ store.GuardFunc) error {
			assert.Len(t, request.NewState.ShardAssignments["exec-1"].AssignedShards, 2)
			assert.Len(t, request.NewState.ShardAssignments["exec-2"].AssignedShards, 1)
			return nil
		},
	)

	err := processor.rebalanceShards(context.Background())
	require.NoError(t, err)
}

func TestRebalanceShards_AppliesGreedyLoadBalancingPlan(t *testing.T) {
	mocks := setupProcessorTest(t, config.NamespaceTypeEphemeral)
	defer mocks.ctrl.Finish()
	initialShardsPerExecutor := 50
	// With 100 shards total, a 1% move budget permits one shard move.
	expectedMovedShards := 1

	mocks.sdConfig.LoadBalancingMode = func(namespace string) string {
		return config.LoadBalancingModeGREEDY
	}
	mocks.sdConfig.LoadBalancingGreedy = config.LoadBalancingGreedyConfig{
		PerShardCooldown: func(namespace string) time.Duration {
			return time.Minute
		},
		MoveBudgetProportion: func(namespace string) float64 {
			return 0.01
		},
		HysteresisUpperBand: func(namespace string) float64 {
			return 1.15
		},
		HysteresisLowerBand: func(namespace string) float64 {
			return 0.90
		},
		SevereImbalanceRatio: func(namespace string) float64 {
			return 1.3
		},
	}
	processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)
	logger, logs := testlogger.NewObserved(t)
	processor.logger = logger

	now := mocks.timeSource.Now()
	// exec-1 has higher smoothed load, so greedy should move one shard to exec-2.
	heartbeats := map[string]store.HeartbeatState{
		"exec-1": {Status: types.ExecutorStatusACTIVE, LastHeartbeat: now},
		"exec-2": {Status: types.ExecutorStatusACTIVE, LastHeartbeat: now},
	}
	assignments := map[string]store.AssignedState{
		"exec-1": {AssignedShards: make(map[string]*types.ShardAssignment)},
		"exec-2": {AssignedShards: make(map[string]*types.ShardAssignment)},
	}
	shardStats := make(map[string]store.ShardStatistics)
	for i := range initialShardsPerExecutor {
		shardID := "a-" + strconv.Itoa(i)
		assignments["exec-1"].AssignedShards[shardID] = &types.ShardAssignment{Status: types.AssignmentStatusREADY}
		shardStats[shardID] = store.ShardStatistics{SmoothedLoad: 3.0, LastUpdateTime: now}
	}
	for i := range initialShardsPerExecutor {
		shardID := "b-" + strconv.Itoa(i)
		assignments["exec-2"].AssignedShards[shardID] = &types.ShardAssignment{Status: types.AssignmentStatusREADY}
		shardStats[shardID] = store.ShardStatistics{SmoothedLoad: 1.0, LastUpdateTime: now}
	}

	mocks.store.EXPECT().GetState(gomock.Any(), mocks.cfg.Name).Return(&store.NamespaceState{
		Executors:        heartbeats,
		ShardAssignments: assignments,
		ShardStats:       shardStats,
	}, nil)
	mocks.election.EXPECT().Guard().Return(store.NopGuard())
	mocks.store.EXPECT().AssignShards(gomock.Any(), mocks.cfg.Name, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, request store.AssignShardsRequest, _ store.GuardFunc) error {
			assert.Len(t, request.NewState.ShardAssignments["exec-1"].AssignedShards, initialShardsPerExecutor-expectedMovedShards)
			assert.Len(t, request.NewState.ShardAssignments["exec-2"].AssignedShards, initialShardsPerExecutor+expectedMovedShards)
			return nil
		},
	)
	mocks.store.EXPECT().RecordShardStatisticsBatch(gomock.Any(), mocks.cfg.Name, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, updates []store.ExecutorShardStatistics) error {
			updatesByExecutor := make(map[string]map[string]store.ShardStatistics, len(updates))
			for _, update := range updates {
				updatesByExecutor[update.ExecutorID] = update.Statistics
			}
			assert.Len(t, updatesByExecutor["exec-1"], initialShardsPerExecutor-expectedMovedShards)
			assert.Len(t, updatesByExecutor["exec-2"], initialShardsPerExecutor+expectedMovedShards)
			return assert.AnError
		},
	)

	err := processor.rebalanceShards(context.Background())
	require.NoError(t, err)
	entries := logs.FilterMessage("failed to record shard statistics after assignment").All()
	require.Len(t, entries, 1)
	assert.Equal(t, zapcore.WarnLevel, entries[0].Level)
}

func TestGetShards_Utility(t *testing.T) {
	t.Run("Fixed type", func(t *testing.T) {
		cfg := config.Namespace{Type: config.NamespaceTypeFixed, ShardNum: 5}
		shards := getShards(cfg, nil, nil)
		assert.Equal(t, []string{"0", "1", "2", "3", "4"}, shards)
	})

	t.Run("Ephemeral type", func(t *testing.T) {
		cfg := config.Namespace{Type: config.NamespaceTypeEphemeral}
		nsState := &store.NamespaceState{
			ShardAssignments: map[string]store.AssignedState{
				"executor1": {
					AssignedShards: map[string]*types.ShardAssignment{
						"s0": {Status: types.AssignmentStatusREADY},
						"s1": {Status: types.AssignmentStatusREADY},
						"s2": {Status: types.AssignmentStatusREADY},
					},
				},
				"executor2": {
					AssignedShards: map[string]*types.ShardAssignment{
						"s3": {Status: types.AssignmentStatusREADY},
						"s4": {Status: types.AssignmentStatusREADY},
					},
				},
			},
		}
		shards := getShards(cfg, nsState, nil)
		slices.Sort(shards)
		assert.Equal(t, []string{"s0", "s1", "s2", "s3", "s4"}, shards)
	})

	t.Run("Ephemeral type with deleted shards", func(t *testing.T) {
		cfg := config.Namespace{Type: config.NamespaceTypeEphemeral}
		nsState := &store.NamespaceState{
			ShardAssignments: map[string]store.AssignedState{
				"executor1": {
					AssignedShards: map[string]*types.ShardAssignment{
						"s0": {Status: types.AssignmentStatusREADY},
						"s1": {Status: types.AssignmentStatusREADY},
						"s2": {Status: types.AssignmentStatusREADY},
					},
				},
				"executor2": {
					AssignedShards: map[string]*types.ShardAssignment{
						"s3": {Status: types.AssignmentStatusREADY},
						"s4": {Status: types.AssignmentStatusREADY},
					},
				},
			},
		}
		deletedShards := map[string]store.ShardState{
			"s0": {},
			"s1": {},
		}
		shards := getShards(cfg, nsState, deletedShards)
		slices.Sort(shards)
		assert.Equal(t, []string{"s2", "s3", "s4"}, shards)
	})

	// Unknown type
	t.Run("Other type", func(t *testing.T) {
		cfg := config.Namespace{Type: "other"}
		shards := getShards(cfg, nil, nil)
		assert.Nil(t, shards)
	})
}

func TestAssignShardsToEmptyExecutors(t *testing.T) {
	cases := []struct {
		name                       string
		inputAssignments           map[string][]string
		expectedAssignments        map[string][]string
		expectedDistributonChanged bool
	}{
		{
			name:                       "no executors",
			inputAssignments:           map[string][]string{},
			expectedAssignments:        map[string][]string{},
			expectedDistributonChanged: false,
		},
		{
			name: "no empty executors",
			inputAssignments: map[string][]string{
				"exec-1": {"shard-1", "shard-2", "shard-3", "shard-4", "shard-5", "shard-6"},
				"exec-2": {"shard-7", "shard-8"},
			},
			expectedAssignments: map[string][]string{
				"exec-1": {"shard-1", "shard-2", "shard-3", "shard-4", "shard-5", "shard-6"},
				"exec-2": {"shard-7", "shard-8"},
			},
			expectedDistributonChanged: false,
		},
		{
			name: "empty executor",
			inputAssignments: map[string][]string{
				"exec-1": {"shard-1", "shard-2", "shard-3", "shard-4", "shard-5", "shard-6"},
				"exec-2": {"shard-7", "shard-8", "shard-9", "shard-10"},
				"exec-3": {},
			},
			expectedAssignments: map[string][]string{
				"exec-1": {"shard-2", "shard-3", "shard-4", "shard-5", "shard-6"},
				"exec-2": {"shard-8", "shard-9", "shard-10"},
				"exec-3": {"shard-1", "shard-7"},
			},
			expectedDistributonChanged: true,
		},
		{
			name:                       "all empty executors",
			inputAssignments:           map[string][]string{"exec-1": {}, "exec-2": {}, "exec-3": {}},
			expectedAssignments:        map[string][]string{"exec-1": {}, "exec-2": {}, "exec-3": {}},
			expectedDistributonChanged: false,
		},
		{
			name: "multiple empty executors",
			inputAssignments: map[string][]string{
				"exec-1": {"shard-1", "shard-2", "shard-3", "shard-4", "shard-5", "shard-6", "shard-7", "shard-8", "shard-9", "shard-10"},
				"exec-2": {"shard-11", "shard-12", "shard-13", "shard-14", "shard-15", "shard-16", "shard-17"},
				"exec-3": {"shard-18", "shard-19", "shard-20", "shard-21", "shard-22", "shard-23", "shard-24"},
				"exec-4": {},
				"exec-5": {},
			},
			expectedAssignments: map[string][]string{
				"exec-1": {"shard-4", "shard-5", "shard-6", "shard-7", "shard-8", "shard-9", "shard-10"},
				"exec-2": {"shard-14", "shard-15", "shard-16", "shard-17"},
				"exec-3": {"shard-20", "shard-21", "shard-22", "shard-23", "shard-24"},
				"exec-4": {"shard-1", "shard-18", "shard-12", "shard-3"},
				"exec-5": {"shard-11", "shard-2", "shard-19", "shard-13"},
			},
			expectedDistributonChanged: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actualDistributionChanged := assignShardsToEmptyExecutors(c.inputAssignments)

			assert.Equal(t, c.expectedAssignments, c.inputAssignments)
			assert.Equal(t, c.expectedDistributonChanged, actualDistributionChanged)
		})
	}
}

func TestApplyMoves(t *testing.T) {
	cases := []struct {
		name           string
		assignments    map[string][]string
		moves          []plan.Move
		expected       map[string][]string
		expectError    bool
		expectedErrMsg string
	}{
		{
			name: "single move",
			assignments: map[string][]string{
				"exec-a": {"shard-1", "shard-2"},
				"exec-b": {"shard-3"},
			},
			moves: []plan.Move{{ShardID: "shard-1", From: "exec-a", To: "exec-b"}},
			expected: map[string][]string{
				"exec-a": {"shard-2"},
				"exec-b": {"shard-3", "shard-1"},
			},
		},
		{
			name: "multiple moves",
			assignments: map[string][]string{
				"exec-a": {"shard-1", "shard-2"},
				"exec-b": {"shard-3", "shard-4"},
			},
			moves: []plan.Move{
				{ShardID: "shard-1", From: "exec-a", To: "exec-b"},
				{ShardID: "shard-3", From: "exec-b", To: "exec-a"},
			},
			// moveShard swaps with the last element, so order may change.
			expected: map[string][]string{
				"exec-a": {"shard-2", "shard-3"},
				"exec-b": {"shard-1", "shard-4"},
			},
		},
		{
			name: "empty moves is a no-op",
			assignments: map[string][]string{
				"exec-a": {"shard-1"},
				"exec-b": {"shard-2"},
			},
			moves: []plan.Move{},
			expected: map[string][]string{
				"exec-a": {"shard-1"},
				"exec-b": {"shard-2"},
			},
		},
		{
			name: "shard not found in source returns error",
			assignments: map[string][]string{
				"exec-a": {"shard-1"},
				"exec-b": {"shard-2"},
			},
			moves:          []plan.Move{{ShardID: "shard-missing", From: "exec-a", To: "exec-b"}},
			expectError:    true,
			expectedErrMsg: "shard shard-missing not found in source executor exec-a",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := applyMoves(c.assignments, c.moves)
			if c.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.expectedErrMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.expected, c.assignments)
		})
	}
}

func TestBuildHandoverStats(t *testing.T) {
	mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
	defer mocks.ctrl.Finish()
	processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

	now := time.Now().UTC()
	shardID := "shard-1"
	newExecutorID := "exec-new"

	type testCase struct {
		name             string
		previousOwner    string
		drained          bool
		executors        map[string]store.HeartbeatState
		expectShardStats *store.ShardHandoverStats // nil means expect no handover stats
	}

	testCases := []testCase{
		{
			name:             "no previous owner -> stat without handover",
			executors:        map[string]store.HeartbeatState{},
			expectShardStats: nil,
		},
		{
			name:          "drained shard -> stat without handover",
			previousOwner: "old-exec",
			drained:       true,
			executors: map[string]store.HeartbeatState{
				"old-exec": {
					Status:        types.ExecutorStatusACTIVE,
					LastHeartbeat: now.Add(-10 * time.Second),
				},
			},
			expectShardStats: nil,
		},
		{
			name:          "same executor as previous -> nil",
			previousOwner: newExecutorID,
			executors: map[string]store.HeartbeatState{
				newExecutorID: {
					Status:        types.ExecutorStatusACTIVE,
					LastHeartbeat: now.Add(-10 * time.Second),
				},
			},
			expectShardStats: nil,
		},
		{
			name:             "prev executor different but heartbeat missing -> no handover",
			previousOwner:    "old-exec",
			executors:        map[string]store.HeartbeatState{},
			expectShardStats: nil,
		},
		{
			name:          "prev executor ACTIVE -> emergency handover",
			previousOwner: "old-active",
			executors: map[string]store.HeartbeatState{
				"old-active": {
					Status:        types.ExecutorStatusACTIVE,
					LastHeartbeat: now.Add(-10 * time.Second),
				},
			},
			expectShardStats: &store.ShardHandoverStats{
				HandoverType:                      types.HandoverTypeEMERGENCY,
				PreviousExecutorLastHeartbeatTime: now.Add(-10 * time.Second),
			},
		},
		{
			name:          "prev executor DRAINING -> graceful handover",
			previousOwner: "old-draining",
			executors: map[string]store.HeartbeatState{
				"old-draining": {
					Status:        types.ExecutorStatusDRAINING,
					LastHeartbeat: now.Add(-10 * time.Second),
				},
			},
			expectShardStats: &store.ShardHandoverStats{
				HandoverType:                      types.HandoverTypeGRACEFUL,
				PreviousExecutorLastHeartbeatTime: now.Add(-10 * time.Second),
			},
		},
		{
			name:          "prev executor DRAINED -> graceful handover",
			previousOwner: "old-drained",
			executors: map[string]store.HeartbeatState{
				"old-drained": {
					Status:        types.ExecutorStatusDRAINED,
					LastHeartbeat: now.Add(-10 * time.Second),
				},
			},
			expectShardStats: &store.ShardHandoverStats{
				HandoverType:                      types.HandoverTypeGRACEFUL,
				PreviousExecutorLastHeartbeatTime: now.Add(-10 * time.Second),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			previousOwners := make(map[string]string)
			if tc.previousOwner != "" {
				previousOwners[shardID] = tc.previousOwner
			}
			drainedShards := make(map[string]struct{})
			if tc.drained {
				drainedShards[shardID] = struct{}{}
			}

			stats := processor.buildHandoverStats(
				&store.NamespaceState{Executors: tc.executors, DrainedShards: drainedShards},
				previousOwners,
				newExecutorID,
				[]string{shardID},
			)
			stat, ok := stats[shardID]
			if tc.expectShardStats == nil {
				require.False(t, ok)
				return
			}
			require.True(t, ok)
			require.Equal(t, *tc.expectShardStats, stat)
		})
	}
}

func TestBuildHandoverStats_MultipleShards(t *testing.T) {
	mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
	defer mocks.ctrl.Finish()
	processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

	now := time.Now().UTC()
	executorID := "exec-1"
	shardIDs := []string{"shard-1", "shard-2"}
	previousOwners := map[string]string{
		"shard-1": "old-active",
	}
	namespaceState := &store.NamespaceState{
		Executors: map[string]store.HeartbeatState{
			"old-active": {
				Status:        types.ExecutorStatusACTIVE,
				LastHeartbeat: now.Add(-10 * time.Second),
			},
		},
	}
	expected := map[string]store.ShardHandoverStats{
		"shard-1": {
			HandoverType:                      types.HandoverTypeEMERGENCY,
			PreviousExecutorLastHeartbeatTime: now.Add(-10 * time.Second),
		},
	}

	stats := processor.buildHandoverStats(namespaceState, previousOwners, executorID, shardIDs)

	assert.Equal(t, expected, stats)
}

func TestBuildNewAssignmentsState_OnlyChangedExecutors(t *testing.T) {
	mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
	processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

	now := mocks.timeSource.Now().UTC()
	oldTime := now.Add(-time.Hour)

	namespaceState := &store.NamespaceState{
		ShardAssignments: map[string]store.AssignedState{
			"exec-1": {
				AssignedShards: map[string]*types.ShardAssignment{
					"shard-1": {Status: types.AssignmentStatusREADY},
					"shard-2": {Status: types.AssignmentStatusREADY},
				},
				LastUpdated: oldTime,
				ModRevision: 10,
			},
			"exec-2": {
				AssignedShards: map[string]*types.ShardAssignment{
					"shard-3": {Status: types.AssignmentStatusREADY},
				},
				LastUpdated: oldTime,
				ModRevision: 20,
			},
			"exec-4": {
				AssignedShards: map[string]*types.ShardAssignment{
					"shard-6": {Status: types.AssignmentStatusREADY},
				},
				LastUpdated: oldTime,
				ModRevision: 30,
			},
		},
	}

	currentAssignments := map[string][]string{
		"exec-1": {"shard-1", "shard-2"}, // unchanged
		"exec-2": {"shard-3", "shard-4"}, // changed (added shard-4)
		"exec-3": {"shard-5"},            // new
	}
	executorsToUnassign := map[string]int64{"exec-4": 30}

	newAssignments, executorsWithChangedAssignments := processor.buildNewAssignmentsState(
		namespaceState,
		currentAssignments,
		executorsToUnassign,
		namespaceState.ShardOwners(),
		now,
	)

	assert.Len(t, newAssignments, 4)
	assert.Equal(t, map[string]struct{}{"exec-2": {}, "exec-3": {}, "exec-4": {}}, executorsWithChangedAssignments)
	assert.Equal(t, oldTime, newAssignments["exec-1"].LastUpdated)
	assert.Equal(t, now, newAssignments["exec-2"].LastUpdated)
	assert.Equal(t, int64(10), newAssignments["exec-1"].ModRevision)
	assert.Equal(t, int64(20), newAssignments["exec-2"].ModRevision)
	assert.Equal(t, int64(0), newAssignments["exec-3"].ModRevision)
	assert.Empty(t, newAssignments["exec-4"].AssignedShards)
	assert.Equal(t, int64(30), newAssignments["exec-4"].ModRevision, "cleared assignments")
}

func TestFindExecutorsToUnassign(t *testing.T) {
	assignment := func(modRevision int64, shards ...string) store.AssignedState {
		assigned := make(map[string]*types.ShardAssignment, len(shards))
		for _, shardID := range shards {
			assigned[shardID] = &types.ShardAssignment{Status: types.AssignmentStatusREADY}
		}
		return store.AssignedState{AssignedShards: assigned, ModRevision: modRevision}
	}

	tests := []struct {
		name       string
		status     types.ExecutorStatus
		assignment store.AssignedState
		stale      bool
		want       map[string]int64
	}{
		{
			name:       "active executor keeps its shards",
			status:     types.ExecutorStatusACTIVE,
			assignment: assignment(7, "0"),
			want:       map[string]int64{},
		},
		{
			name:       "draining executor is emptied at its current revision",
			status:     types.ExecutorStatusDRAINING,
			assignment: assignment(7, "0"),
			want:       map[string]int64{"exec": 7},
		},
		{
			name:       "drained executor is emptied",
			status:     types.ExecutorStatusDRAINED,
			assignment: assignment(7, "0"),
			want:       map[string]int64{"exec": 7},
		},
		{
			name:       "stale executor is deleted rather than emptied",
			status:     types.ExecutorStatusACTIVE,
			assignment: assignment(7, "0"),
			stale:      true,
			want:       map[string]int64{},
		},
		{
			name:       "already emptied record is not rewritten",
			status:     types.ExecutorStatusDRAINING,
			assignment: assignment(7),
			want:       map[string]int64{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			staleExecutors := map[string]int64{}
			if tt.stale {
				staleExecutors["exec"] = tt.assignment.ModRevision
			}

			namespaceState := &store.NamespaceState{
				Executors:        map[string]store.HeartbeatState{"exec": {Status: tt.status}},
				ShardAssignments: map[string]store.AssignedState{"exec": tt.assignment},
			}

			assert.Equal(t, tt.want, findExecutorsToUnassign(namespaceState, staleExecutors))
		})
	}
}

func TestEmitExecutorMetric(t *testing.T) {
	tests := []struct {
		name           string
		executors      map[string]store.HeartbeatState
		expectedCounts map[types.ExecutorStatus]int
	}{
		{
			name:           "empty executors",
			executors:      map[string]store.HeartbeatState{},
			expectedCounts: map[types.ExecutorStatus]int{},
		},
		{
			name: "single active executor",
			executors: map[string]store.HeartbeatState{
				"exec-1": {Status: types.ExecutorStatusACTIVE},
			},
			expectedCounts: map[types.ExecutorStatus]int{
				types.ExecutorStatusACTIVE: 1,
			},
		},
		{
			name: "multiple executors",
			executors: map[string]store.HeartbeatState{
				"exec-1": {Status: types.ExecutorStatusACTIVE},
				"exec-2": {Status: types.ExecutorStatusACTIVE},
				"exec-3": {Status: types.ExecutorStatusDRAINING},
				"exec-4": {Status: types.ExecutorStatusDRAINED},
			},
			expectedCounts: map[types.ExecutorStatus]int{
				types.ExecutorStatusACTIVE:   2,
				types.ExecutorStatusDRAINING: 1,
				types.ExecutorStatusDRAINED:  1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
			defer mocks.ctrl.Finish()
			processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

			namespaceState := &store.NamespaceState{
				Executors: tt.executors,
			}

			metricsScope := &metricmocks.Scope{}

			for status, count := range tt.expectedCounts {
				taggedScope := &metricmocks.Scope{}
				metricsScope.On("Tagged", metrics.ExecutorStatusTag(status.String())).Return(taggedScope).Once()
				taggedScope.On("UpdateGauge", metrics.ShardDistributorTotalExecutors, float64(count)).Once()
			}

			processor.emitExecutorMetric(namespaceState, metricsScope)

			metricsScope.AssertExpectations(t)
		})
	}
}

func TestEmitOldestExecutorHeartbeatLag(t *testing.T) {
	tests := []struct {
		name        string
		executors   map[string]store.HeartbeatState
		expectedLag *float64
	}{
		{
			name:        "empty executors",
			executors:   map[string]store.HeartbeatState{},
			expectedLag: nil,
		},
		{
			name: "single executor",
			executors: map[string]store.HeartbeatState{
				"exec-1": {Status: types.ExecutorStatusACTIVE},
			},
			expectedLag: common.Float64Ptr(5000),
		},
		{
			name: "multiple executors",
			executors: map[string]store.HeartbeatState{
				"exec-1": {Status: types.ExecutorStatusACTIVE},   // 5 seconds
				"exec-2": {Status: types.ExecutorStatusACTIVE},   // 10 seconds (oldest)
				"exec-3": {Status: types.ExecutorStatusDRAINING}, // 3 seconds
			},
			expectedLag: common.Float64Ptr(10000), // 10 seconds
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
			defer mocks.ctrl.Finish()
			processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

			now := mocks.timeSource.Now()

			if tt.name == "single executor" {
				tt.executors["exec-1"] = store.HeartbeatState{
					Status:        types.ExecutorStatusACTIVE,
					LastHeartbeat: now.Add(-5 * time.Second),
				}
			} else if tt.name == "multiple executors" {
				tt.executors["exec-1"] = store.HeartbeatState{
					Status:        types.ExecutorStatusACTIVE,
					LastHeartbeat: now.Add(-5 * time.Second),
				}
				tt.executors["exec-2"] = store.HeartbeatState{
					Status:        types.ExecutorStatusACTIVE,
					LastHeartbeat: now.Add(-10 * time.Second), // oldest
				}
				tt.executors["exec-3"] = store.HeartbeatState{
					Status:        types.ExecutorStatusDRAINING,
					LastHeartbeat: now.Add(-3 * time.Second),
				}
			}

			namespaceState := &store.NamespaceState{
				Executors: tt.executors,
			}

			metricsScope := &metricmocks.Scope{}

			if tt.expectedLag != nil {
				metricsScope.On("UpdateGauge", metrics.ShardDistributorOldestExecutorHeartbeatLag, *tt.expectedLag).Once()
			}

			processor.emitOldestExecutorHeartbeatLag(namespaceState, metricsScope)

			metricsScope.AssertExpectations(t)
		})
	}
}

func TestEmitMaxOwnersPerShardMetric(t *testing.T) {
	tests := []struct {
		name                 string
		shardAssignments     map[string]store.AssignedState
		expectedMaxExecutors float64
		expectedErrorShards  map[string][]string
	}{
		{
			name:                 "no shards",
			shardAssignments:     map[string]store.AssignedState{},
			expectedMaxExecutors: 0,
		},
		{
			name: "single shard, single executor",
			shardAssignments: map[string]store.AssignedState{
				"exec-1": {
					AssignedShards: map[string]*types.ShardAssignment{
						"shard-1": {Status: types.AssignmentStatusREADY},
					},
				},
			},
			expectedMaxExecutors: 1,
		},
		{
			name: "multiple executors, different shard counts",
			shardAssignments: map[string]store.AssignedState{
				"exec-1": {
					AssignedShards: map[string]*types.ShardAssignment{
						"shard-1": {Status: types.AssignmentStatusREADY},
						"shard-2": {Status: types.AssignmentStatusREADY},
						"shard-3": {Status: types.AssignmentStatusREADY},
					},
				},
				"exec-2": {
					AssignedShards: map[string]*types.ShardAssignment{
						"shard-4": {Status: types.AssignmentStatusREADY},
					},
				},
			},
			expectedMaxExecutors: 1,
		},
		{
			name: "multiple executors, multiple owners per shard",
			shardAssignments: map[string]store.AssignedState{
				"exec-1": {
					AssignedShards: map[string]*types.ShardAssignment{
						"shard-1": {Status: types.AssignmentStatusREADY},
						"shard-2": {Status: types.AssignmentStatusREADY},
						"shard-3": {Status: types.AssignmentStatusREADY},
					},
				},
				"exec-2": {
					AssignedShards: map[string]*types.ShardAssignment{
						"shard-2": {Status: types.AssignmentStatusREADY},
						"shard-3": {Status: types.AssignmentStatusREADY},
						"shard-4": {Status: types.AssignmentStatusREADY},
					},
				},
			},
			expectedErrorShards: map[string][]string{
				"shard-2": {"exec-1", "exec-2"},
				"shard-3": {"exec-1", "exec-2"},
			},
			expectedMaxExecutors: 2,
		},
		{
			name: "multi-owned shard below the max owner count",
			shardAssignments: map[string]store.AssignedState{
				"exec-1": {
					AssignedShards: map[string]*types.ShardAssignment{
						"shard-1": {Status: types.AssignmentStatusREADY},
						"shard-2": {Status: types.AssignmentStatusREADY},
					},
				},
				"exec-2": {
					AssignedShards: map[string]*types.ShardAssignment{
						"shard-1": {Status: types.AssignmentStatusREADY},
						"shard-2": {Status: types.AssignmentStatusREADY},
					},
				},
				"exec-3": {
					AssignedShards: map[string]*types.ShardAssignment{
						"shard-1": {Status: types.AssignmentStatusREADY},
					},
				},
			},
			expectedErrorShards: map[string][]string{
				"shard-1": {"exec-1", "exec-2", "exec-3"},
				"shard-2": {"exec-1", "exec-2"},
			},
			expectedMaxExecutors: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
			defer mocks.ctrl.Finish()
			processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

			metricsScope := &metricmocks.Scope{}
			metricsScope.On("UpdateGauge", metrics.ShardDistributorMaxExecutorsPerShard, tt.expectedMaxExecutors).Once()

			loggerMock := log.NewMockLogger(mocks.ctrl)
			processor.logger = loggerMock
			expectedLogMessage := "shard owned by multiple executors"
			for shardID, executors := range tt.expectedErrorShards {
				loggerMock.EXPECT().Error(expectedLogMessage,
					tag.ShardKey(shardID),
					tag.ShardExecutors(executors),
					tag.ShardNamespace("test-ns"),
				).Return()
			}

			processor.emitMaxOwnersPerShardMetric(tt.shardAssignments, metricsScope)

			metricsScope.AssertExpectations(t)
		})
	}
}

func TestRunRebalanceTriggeringLoop(t *testing.T) {
	t.Run("no events from subscribe, trigger from ticker", func(t *testing.T) {
		mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
		defer mocks.ctrl.Finish()
		processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		updateChan := make(chan int64)
		triggerChan := make(chan string, 1)

		go processor.rebalanceTriggeringLoop(ctx, updateChan, triggerChan)

		// Wait for ticker to be created
		mocks.timeSource.BlockUntil(1)

		// Advance time to trigger the ticker
		mocks.timeSource.Advance(processor.cfg.Period)

		// Expect trigger from periodic reconciliation
		select {
		case reason := <-triggerChan:
			assert.Equal(t, "Periodic reconciliation triggered", reason)
		case <-time.After(time.Second):
			t.Fatal("expected trigger from ticker, but timed out")
		}

		cancel()
	})

	t.Run("events from subscribe before period, trigger from state change", func(t *testing.T) {
		mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
		defer mocks.ctrl.Finish()
		processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		updateChan := make(chan int64, 1)
		triggerChan := make(chan string, 1)

		go processor.rebalanceTriggeringLoop(ctx, updateChan, triggerChan)

		// Wait for ticker to be created
		mocks.timeSource.BlockUntil(1)

		// Send a state change event before the ticker fires
		updateChan <- 1

		// Expect trigger from state change
		select {
		case reason := <-triggerChan:
			assert.Equal(t, "State change detected", reason)
		case <-time.After(time.Second):
			t.Fatal("expected trigger from state change, but timed out")
		}

		cancel()
	})

	t.Run("triggerChan full, multiple subscribe events, loop not stuck", func(t *testing.T) {
		mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
		defer mocks.ctrl.Finish()
		processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

		ctx, cancel := context.WithCancel(context.Background())

		// Use unbuffered channel for updates to ensure they are processed one at a time
		updateChan := make(chan int64)
		triggerChan := make(chan string, 1)

		var loopWg sync.WaitGroup
		loopWg.Add(1)
		go func() {
			defer loopWg.Done()
			processor.rebalanceTriggeringLoop(ctx, updateChan, triggerChan)
		}()
		defer func() {
			cancel()
			loopWg.Wait()
		}()

		// Wait for ticker to be created
		mocks.timeSource.BlockUntil(1)

		// Don't read from triggerChan yet to keep it full
		// Send multiple state change events
		for i := int64(0); i <= 10; i++ {
			select {
			case updateChan <- i:
			case <-time.After(time.Second):
				// Expect that the loop is not stuck
				t.Fatalf("failed to send update %d, channel blocked", i)
			}
		}

		// Expect trigger from state change
		select {
		case reason := <-triggerChan:
			assert.Equal(t, "State change detected", reason)
		case <-time.After(time.Second):
			t.Fatal("expected trigger from state change, but timed out")
		}
	})

	t.Run("update channel closed stops loop", func(t *testing.T) {
		mocks := setupProcessorTest(t, config.NamespaceTypeFixed)
		defer mocks.ctrl.Finish()
		processor := mocks.factory.CreateProcessor(mocks.cfg, mocks.store, mocks.election).(*namespaceProcessor)

		ctx := context.Background()

		updateChan := make(chan int64)
		triggerChan := make(chan string, 1)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			processor.rebalanceTriggeringLoop(ctx, updateChan, triggerChan)
		}()

		// Wait for ticker to be created
		mocks.timeSource.BlockUntil(1)

		// Close update channel
		close(updateChan)

		// Wait for loop to exit
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Loop exited as expected
		case <-time.After(time.Second):
			t.Fatal("loop did not exit after updateChan closed")
		}
	})
}
