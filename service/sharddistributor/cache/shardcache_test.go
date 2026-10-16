package cache

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/namespaces"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

func TestShardExecutorCacheForwarding(t *testing.T) {
	const namespace = "test-namespace"
	metadata := map[string]string{
		"datacenter": "dc1",
		"rack":       "rack-42",
	}

	executorStore := store.NewMockStore(gomock.NewController(t))
	executorStore.EXPECT().
		SubscribeToNamespaceChanges(namespace).
		Return((<-chan struct{})(make(chan struct{})), nil)
	executorStore.EXPECT().
		GetAssignmentState(gomock.Any(), namespace).
		Return(namespaceState(1, map[string]testExecutor{
			"executor-1": {shards: []string{"shard-1"}, metadata: metadata},
		}), nil)

	lc := fxtest.NewLifecycle(t)
	cache := newTestShardCache(t, lc, executorStore, namespace).(*ShardToExecutorCache)
	lc.RequireStart()
	defer lc.RequireStop()

	// Read through as the cache is empty
	owner, err := cache.GetShardOwner(context.Background(), namespace, "shard-1")
	require.NoError(t, err)
	assert.Equal(t, "executor-1", owner.ExecutorID)
	assert.Equal(t, metadata, owner.Metadata)

	// Check the cache is populated
	assert.Equal(t, int64(1), cache.namespaceToShards[namespace].executorRevision["executor-1"])
	assert.Equal(t, "executor-1", cache.namespaceToShards[namespace].shardToExecutor["shard-1"].ExecutorID)

	// Check the executor is also cached
	executor, err := cache.GetExecutor(context.Background(), namespace, "executor-1")
	require.NoError(t, err)
	assert.Equal(t, "executor-1", executor.ExecutorID)
	assert.Equal(t, metadata, executor.Metadata)

	assignments, err := cache.GetShardAssignments(namespace)
	require.NoError(t, err)
	assert.Equal(t, map[*store.ShardOwner][]string{executor: {"shard-1"}}, assignments.ExecutorToShards)

	notifyCh, unsubscribe, err := cache.Subscribe(namespace)
	require.NoError(t, err)
	require.NotNil(t, notifyCh)
	unsubscribe()
}

// A failed start must stop the namespaces that did start.
func TestShardCacheStartFailure(t *testing.T) {
	defer goleak.VerifyNone(t)

	executorStore := store.NewMockStore(gomock.NewController(t))
	executorStore.EXPECT().
		SubscribeToNamespaceChanges(gomock.Any()).
		Return((<-chan struct{})(make(chan struct{})), nil).
		MaxTimes(1)
	executorStore.EXPECT().
		SubscribeToNamespaceChanges(gomock.Any()).
		Return(nil, errors.New("subscribe failed"))

	lc := fxtest.NewLifecycle(t)
	newTestShardCache(t, lc, executorStore, "ns-1", "ns-2")

	require.ErrorContains(t, lc.Start(context.Background()), "subscribe failed")
}

func TestShardCacheUnknownNamespace(t *testing.T) {
	// No expectations: an unknown namespace must not reach the store.
	executorStore := store.NewMockStore(gomock.NewController(t))
	lc := fxtest.NewLifecycle(t)
	cache := newTestShardCache(t, lc, executorStore).(*ShardToExecutorCache)
	lc.RequireStart()
	defer lc.RequireStop()

	const namespace = "unknown"
	tests := map[string]func() error{
		"GetShardOwner": func() error {
			_, err := cache.GetShardOwner(context.Background(), namespace, "shard-1")
			return err
		},
		"IsShardDrained": func() error {
			_, err := cache.IsShardDrained(context.Background(), namespace, "shard-1")
			return err
		},
		"GetExecutor": func() error {
			_, err := cache.GetExecutor(context.Background(), namespace, "executor-1")
			return err
		},
		"GetShardAssignments": func() error {
			_, err := cache.GetShardAssignments(namespace)
			return err
		},
		"Subscribe": func() error {
			_, _, err := cache.Subscribe(namespace)
			return err
		},
	}

	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			var notFound *types.NamespaceNotFoundError
			require.ErrorAs(t, call(), &notFound)
		})
	}
}

func newTestShardCache(t *testing.T, lc fx.Lifecycle, executorStore store.Store, namespaceNames ...string) ShardCache {
	cfg := config.ShardDistribution{}
	for _, name := range namespaceNames {
		cfg.Namespaces = append(cfg.Namespaces, config.Namespace{Name: name, Type: config.NamespaceTypeEphemeral})
	}
	return NewShardCache(ShardCacheParams{
		Namespaces:    namespaces.MustNewRegistry(cfg),
		ExecutorStore: executorStore,
		Lifecycle:     lc,
		Logger:        testlogger.New(t),
		TimeSource:    clock.NewRealTimeSource(),
		MetricsClient: metrics.NewNoopMetricsClient(),
	})
}
