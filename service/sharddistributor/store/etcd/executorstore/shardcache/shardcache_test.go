package shardcache

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

func TestNewShardCache(t *testing.T) {
	logger := testlogger.New(t)
	executorStore := store.NewMockStore(gomock.NewController(t))

	shardCache := NewShardCache(ShardCacheParams{
		ExecutorStore: executorStore,
		Lifecycle:     fxtest.NewLifecycle(t),
		Logger:        logger,
		TimeSource:    clock.NewRealTimeSource(),
		MetricsClient: metrics.NewNoopMetricsClient(),
	})
	require.NotNil(t, shardCache)

	cache, ok := shardCache.(*ShardToExecutorCache)
	require.True(t, ok)

	assert.NotNil(t, cache.namespaceToShards)
	assert.NotNil(t, cache.stopC)
	assert.Equal(t, logger, cache.logger)
	assert.Equal(t, executorStore, cache.executorStore)
}

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
		GetState(gomock.Any(), namespace).
		Return(namespaceState(1, map[string]testExecutor{
			"executor-1": {shards: []string{"shard-1"}, metadata: metadata},
		}), nil)

	cache := NewShardCache(ShardCacheParams{
		ExecutorStore: executorStore,
		Lifecycle:     fxtest.NewLifecycle(t),
		Logger:        testlogger.New(t),
		TimeSource:    clock.NewRealTimeSource(),
		MetricsClient: metrics.NewNoopMetricsClient(),
	}).(*ShardToExecutorCache)
	cache.Start()
	defer cache.Stop()

	// The namespace is created on first use, and read through as the cache is empty
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

// A namespace that cannot be started must surface the failure on every entry point
// rather than serving an empty cache as if the namespace were simply unknown.
func TestShardCacheNamespaceStartFailure(t *testing.T) {
	const namespace = "test-namespace"

	tests := []struct {
		name string
		call func(*ShardToExecutorCache) error
	}{
		{
			name: "GetShardOwner",
			call: func(c *ShardToExecutorCache) error {
				_, err := c.GetShardOwner(context.Background(), namespace, "shard-1")
				return err
			},
		},
		{
			name: "IsShardDrained",
			call: func(c *ShardToExecutorCache) error {
				_, err := c.IsShardDrained(context.Background(), namespace, "shard-1")
				return err
			},
		},
		{
			name: "GetExecutor",
			call: func(c *ShardToExecutorCache) error {
				_, err := c.GetExecutor(context.Background(), namespace, "executor-1")
				return err
			},
		},
		{
			name: "GetShardAssignments",
			call: func(c *ShardToExecutorCache) error {
				_, err := c.GetShardAssignments(namespace)
				return err
			},
		},
		{
			name: "Subscribe",
			call: func(c *ShardToExecutorCache) error {
				_, _, err := c.Subscribe(namespace)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executorStore := store.NewMockStore(gomock.NewController(t))
			executorStore.EXPECT().
				SubscribeToNamespaceChanges(namespace).
				Return(nil, errors.New("subscribe failed"))

			cache := NewShardCache(ShardCacheParams{
				ExecutorStore: executorStore,
				Lifecycle:     fxtest.NewLifecycle(t),
				Logger:        testlogger.New(t),
				TimeSource:    clock.NewRealTimeSource(),
				MetricsClient: metrics.NewNoopMetricsClient(),
			}).(*ShardToExecutorCache)
			cache.Start()
			defer cache.Stop()

			assert.ErrorContains(t, tt.call(cache), "subscribe failed")
		})
	}
}
