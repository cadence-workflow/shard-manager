package shardcache

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdkeys"
)

func TestNamespaceShardToExecutor_IsShardDrained(t *testing.T) {
	tests := []struct {
		name        string
		drained     []string
		shardID     string
		wantDrained bool
	}{
		{
			name:        "drained shard",
			drained:     []string{"shard-1", "shard-2"},
			shardID:     "shard-1",
			wantDrained: true,
		},
		{
			name:        "shard outside the drained set",
			drained:     []string{"shard-1"},
			shardID:     "shard-2",
			wantDrained: false,
		},
		{
			name:        "nothing drained",
			shardID:     "shard-1",
			wantDrained: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := setupNamespaceShardToExecutorTestCase(t)
			defer tc.ctrl.Finish()
			defer close(tc.stopCh)

			tc.expectEmptyExecutorState()
			tc.setDrainedShards(tt.drained...)

			drained, err := tc.e.IsShardDrained(context.Background(), tt.shardID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDrained, drained)
		})
	}
}

// An empty drained set is indistinguishable from one that was never loaded, so the
// first lookup must read etcd. Every later lookup has to be served from the cache.
func TestNamespaceShardToExecutor_IsShardDrained_ReadsEtcdOnceForEmptySet(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer tc.ctrl.Finish()
	defer close(tc.stopCh)

	tc.expectEmptyExecutorState()

	for range 5 {
		drained, err := tc.e.IsShardDrained(context.Background(), "shard-1")
		require.NoError(t, err)
		require.False(t, drained)
	}

	assert.EqualValues(t, 1, tc.drainedGetCalls.Load(), "drained set should be loaded once and then cached")
}

func TestNamespaceShardToExecutor_IsShardDrained_RefreshError(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer tc.ctrl.Finish()
	defer close(tc.stopCh)

	tc.etcdClient.EXPECT().
		Get(gomock.Any(), tc.executorPrefix, gomock.Any()).
		Return(nil, assert.AnError).
		AnyTimes()

	_, err := tc.e.IsShardDrained(context.Background(), "shard-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

// A drain committed after the cache was populated must reach the cache through the
// drained-shards watch, without any reader asking for it.
func TestNamespaceShardToExecutor_DrainedShardsWatchTriggersRefresh(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer tc.ctrl.Finish()

	tc.expectEmptyExecutorState()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tc.e.namespaceRefreshLoop()
	}()

	// Populate the cache first so the watch, not a cache miss, is what picks the
	// drain up.
	drained, err := tc.e.IsShardDrained(context.Background(), "shard-1")
	require.NoError(t, err)
	require.False(t, drained)

	tc.setDrainedShards("shard-1")
	tc.drainedWatchChan <- clientv3.WatchResponse{
		Events: []*clientv3.Event{
			{
				Type: clientv3.EventTypePut,
				Kv:   &mvccpb.KeyValue{Key: []byte(etcdkeys.BuildDrainedShardKey(tc.prefix, tc.namespace, "shard-1"))},
			},
		},
	}

	assert.Eventually(t, func() bool {
		drained, err := tc.e.IsShardDrained(context.Background(), "shard-1")
		return err == nil && drained
	}, 5*time.Second, 10*time.Millisecond, "drained shard should reach the cache through the watch")

	close(tc.stopCh)
	wg.Wait()
}

// A single unparseable key must not hide the rest of the drained set.
func TestNamespaceShardToExecutor_RefreshDrainedShardsSkipsMalformedKeys(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer tc.ctrl.Finish()
	defer close(tc.stopCh)

	tc.expectEmptyExecutorState()
	// A key directly under the drained prefix carries no shard ID.
	tc.malformedDrainedKey = etcdkeys.BuildDrainedShardsPrefix(tc.prefix, tc.namespace)
	tc.setDrainedShards("shard-1")

	require.NoError(t, tc.e.refreshDrainedShards(context.Background()))

	drained, err := tc.e.IsShardDrained(context.Background(), "shard-1")
	require.NoError(t, err)
	assert.True(t, drained)

	tc.e.RLock()
	defer tc.e.RUnlock()
	assert.Len(t, tc.e.drainedShards, 1, "malformed key should be skipped, not cached")
}

func TestNamespaceShardToExecutor_ReplaceDrainedShards_IgnoresStaleRevision(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer tc.ctrl.Finish()
	defer close(tc.stopCh)

	tc.e.replaceDrainedShards(10, map[string]struct{}{"shard-1": {}})

	// An older snapshot must not undo the newer one.
	tc.e.replaceDrainedShards(5, map[string]struct{}{"shard-2": {}})

	drained, loaded := tc.e.lookupDrained("shard-1")
	assert.True(t, loaded)
	assert.True(t, drained)

	drained, _ = tc.e.lookupDrained("shard-2")
	assert.False(t, drained)

	// A newer snapshot replaces the set wholesale.
	tc.e.replaceDrainedShards(20, map[string]struct{}{"shard-2": {}})

	drained, _ = tc.e.lookupDrained("shard-1")
	assert.False(t, drained)
	drained, _ = tc.e.lookupDrained("shard-2")
	assert.True(t, drained)
}

func TestNamespaceShardToExecutor_hasDrainedShardsChanged(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer tc.ctrl.Finish()
	defer close(tc.stopCh)

	drainedKey := etcdkeys.BuildDrainedShardKey(tc.prefix, tc.namespace, "shard-1")
	executorKey := etcdkeys.BuildExecutorKey(tc.prefix, tc.namespace, tc.executorID, etcdkeys.ExecutorAssignedStateKey)

	tests := []struct {
		name string
		resp clientv3.WatchResponse
		want bool
	}{
		{
			name: "no events",
			resp: clientv3.WatchResponse{},
			want: false,
		},
		{
			name: "drain",
			resp: clientv3.WatchResponse{Events: []*clientv3.Event{
				{Type: clientv3.EventTypePut, Kv: &mvccpb.KeyValue{Key: []byte(drainedKey)}},
			}},
			want: true,
		},
		{
			// Both a drain and an undrain leave an empty value, so an undrain must
			// not be mistaken for an unchanged key.
			name: "undrain with empty previous value",
			resp: clientv3.WatchResponse{Events: []*clientv3.Event{
				{
					Type:   clientv3.EventTypeDelete,
					Kv:     &mvccpb.KeyValue{Key: []byte(drainedKey)},
					PrevKv: &mvccpb.KeyValue{Key: []byte(drainedKey)},
				},
			}},
			want: true,
		},
		{
			name: "key from another keyspace",
			resp: clientv3.WatchResponse{Events: []*clientv3.Event{
				{Type: clientv3.EventTypePut, Kv: &mvccpb.KeyValue{Key: []byte(executorKey)}},
			}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tc.e.hasDrainedShardsChanged(tt.resp))
		})
	}
}
