package shardcache

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/api/v3/etcdserverpb"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
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

			seedDrainedShards(tc.e, 1, tt.drained...)

			drained, err := tc.e.IsShardDrained(context.Background(), tt.shardID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDrained, drained)
		})
	}
}

func TestNamespaceShardToExecutor_IsShardDrained_RefreshError(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer tc.ctrl.Finish()
	defer close(tc.stopCh)

	tc.etcdClient.EXPECT().
		Get(gomock.Any(), tc.namespacePrefix, gomock.Any()).
		Return(nil, assert.AnError).
		AnyTimes()

	_, err := tc.e.IsShardDrained(context.Background(), "shard-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

// A single unparseable key must not hide the rest of the drained set.
func TestNamespaceShardToExecutor_RefreshDrainedShardsSkipsMalformedKeys(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer tc.ctrl.Finish()
	defer close(tc.stopCh)

	drainedKey := etcdkeys.BuildDrainedShardKey(tc.prefix, tc.namespace, "shard-1")
	// A key directly under the drained prefix carries no shard ID.
	malformedKey := etcdkeys.BuildDrainedShardsPrefix(tc.prefix, tc.namespace)

	tc.etcdClient.EXPECT().
		Get(gomock.Any(), tc.namespacePrefix, gomock.Any()).
		Return(&clientv3.GetResponse{
			Header: &etcdserverpb.ResponseHeader{Revision: 1},
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte(malformedKey)},
				{Key: []byte(drainedKey)},
			},
		}, nil)

	drained, err := tc.e.IsShardDrained(context.Background(), "shard-1")
	require.NoError(t, err)
	assert.True(t, drained)

	tc.e.RLock()
	defer tc.e.RUnlock()
	assert.Len(t, tc.e.drainedShards, 1, "malformed key should be skipped, not cached")
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

func seedDrainedShards(e *namespaceShardToExecutor, revision int64, shards ...string) {
	drained := make(map[string]struct{}, len(shards))
	for _, shardID := range shards {
		drained[shardID] = struct{}{}
	}
	e.replaceNamespaceState(
		revision,
		map[string]*store.ShardOwner{},
		map[*store.ShardOwner][]string{},
		map[string]int64{},
		map[string]*store.ShardOwner{},
		drained,
	)
}
