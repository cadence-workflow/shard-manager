package cache

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

func TestShardCacheForwarding(t *testing.T) {
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

	cache := newTestShardCache(t, namespace, executorStore)
	require.NoError(t, cache.Start())
	defer cache.Stop()

	// Read through as the cache is empty
	owner, err := cache.GetShardOwner(context.Background(), "shard-1")
	require.NoError(t, err)
	assert.Equal(t, "executor-1", owner.ExecutorID)
	assert.Equal(t, metadata, owner.Metadata)

	// Check the cache is populated
	assert.Equal(t, int64(1), cache.namespace.executorRevision["executor-1"])
	assert.Equal(t, "executor-1", cache.namespace.shardToExecutor["shard-1"].ExecutorID)

	// Check the executor is also cached
	executor, err := cache.GetExecutor(context.Background(), "executor-1")
	require.NoError(t, err)
	assert.Equal(t, "executor-1", executor.ExecutorID)
	assert.Equal(t, metadata, executor.Metadata)

	assignments := cache.GetShardAssignments()
	assert.Equal(t, map[*store.ShardOwner][]string{executor: {"shard-1"}}, assignments.ExecutorToShards)

	notifyCh, unsubscribe := cache.Subscribe()
	assert.NotNil(t, notifyCh)
	unsubscribe()
}

func TestShardCacheStartFailure(t *testing.T) {
	executorStore := store.NewMockStore(gomock.NewController(t))
	executorStore.EXPECT().
		SubscribeToNamespaceChanges("test-namespace").
		Return(nil, errors.New("subscribe failed"))

	cache := newTestShardCache(t, "test-namespace", executorStore)

	require.ErrorContains(t, cache.Start(), "subscribe failed")
}

func newTestShardCache(t *testing.T, namespace string, executorStore store.Store) *ShardCache {
	return NewShardCache(namespace, executorStore, testlogger.New(t), clock.NewRealTimeSource(), metrics.NewNoopMetricsClient())
}
