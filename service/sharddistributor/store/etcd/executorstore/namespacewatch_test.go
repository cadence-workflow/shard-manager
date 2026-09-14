package executorstore

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdclient"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdkeys"
)

const (
	_watchTestPrefix    = "/test-prefix"
	_watchTestNamespace = "test-namespace"
	_watchTestExecutor  = "executor-1"
)

// newNamespaceWatchStore builds a store wired to a mock etcd client, with the watch
// context the watch goroutines are bounded by.
func newNamespaceWatchStore(t *testing.T, client etcdclient.Client, timeSource clock.TimeSource) *executorStoreImpl {
	t.Helper()

	watchCtx, cancelWatch := context.WithCancel(context.Background())
	t.Cleanup(cancelWatch)

	return &executorStoreImpl{
		client:        client,
		prefix:        _watchTestPrefix,
		logger:        testlogger.New(t),
		timeSource:    timeSource,
		metricsClient: metrics.NewNoopMetricsClient(),
		watchCtx:      watchCtx,
		cancelWatch:   cancelWatch,
	}
}

// The namespace is watched as a single range, so the store must refresh for drain changes,
// ignore the keyspaces subscribers do not track, and treat unchanged rewrites as no-ops.
func TestHasNamespaceStateChanged(t *testing.T) {
	s := newNamespaceWatchStore(t, etcdclient.NewMockClient(gomock.NewController(t)), clock.NewRealTimeSource())

	drainedKey := etcdkeys.BuildDrainedShardKey(_watchTestPrefix, _watchTestNamespace, "shard-1")
	leaderKey := fmt.Sprintf("%s/%s/leader/1234", _watchTestPrefix, _watchTestNamespace)
	assignedStateKey := etcdkeys.BuildExecutorKey(_watchTestPrefix, _watchTestNamespace, _watchTestExecutor, etcdkeys.ExecutorAssignedStateKey)
	reportedShardsKey := etcdkeys.BuildExecutorKey(_watchTestPrefix, _watchTestNamespace, _watchTestExecutor, etcdkeys.ExecutorReportedShardsKey)
	metadataKey := etcdkeys.BuildMetadataKey(_watchTestPrefix, _watchTestNamespace, _watchTestExecutor, "metadata-key")

	tests := []struct {
		name     string
		events   []*clientv3.Event
		expected bool
	}{
		{
			name:     "no events",
			events:   nil,
			expected: false,
		},
		{
			name: "shard drained",
			events: []*clientv3.Event{
				{Type: clientv3.EventTypePut, Kv: &mvccpb.KeyValue{Key: []byte(drainedKey)}},
			},
			expected: true,
		},
		{
			// Drain and undrain both store an empty value, so this must not be mistaken
			// for an unchanged key.
			name: "shard undrained",
			events: []*clientv3.Event{
				{
					Type:   clientv3.EventTypeDelete,
					Kv:     &mvccpb.KeyValue{Key: []byte(drainedKey)},
					PrevKv: &mvccpb.KeyValue{Key: []byte(drainedKey)},
				},
			},
			expected: true,
		},
		{
			name: "leader election key is not tracked",
			events: []*clientv3.Event{
				{Type: clientv3.EventTypePut, Kv: &mvccpb.KeyValue{Key: []byte(leaderKey), Value: []byte("host-1")}},
			},
			expected: false,
		},
		{
			name: "malformed drained shard key",
			events: []*clientv3.Event{
				{Type: clientv3.EventTypePut, Kv: &mvccpb.KeyValue{Key: []byte(etcdkeys.BuildDrainedShardsPrefix(_watchTestPrefix, _watchTestNamespace) + "shard/1")}},
			},
			expected: false,
		},
		{
			name: "executor assigned state changed",
			events: []*clientv3.Event{
				{
					Type:   clientv3.EventTypePut,
					Kv:     &mvccpb.KeyValue{Key: []byte(assignedStateKey), Value: []byte("new")},
					PrevKv: &mvccpb.KeyValue{Key: []byte(assignedStateKey), Value: []byte("old")},
				},
			},
			expected: true,
		},
		{
			name: "executor assigned state rewritten with same value",
			events: []*clientv3.Event{
				{
					Type:   clientv3.EventTypePut,
					Kv:     &mvccpb.KeyValue{Key: []byte(assignedStateKey), Value: []byte("same")},
					PrevKv: &mvccpb.KeyValue{Key: []byte(assignedStateKey), Value: []byte("same")},
				},
			},
			expected: false,
		},
		{
			// Heartbeat traffic is the highest-volume write in the namespace and no
			// subscriber reads it, so it must never cost a re-read.
			name: "executor reported shards is not tracked",
			events: []*clientv3.Event{
				{Type: clientv3.EventTypePut, Kv: &mvccpb.KeyValue{Key: []byte(reportedShardsKey), Value: []byte("reported")}},
			},
			expected: false,
		},
		{
			name: "executor metadata changed",
			events: []*clientv3.Event{
				{
					Type:   clientv3.EventTypePut,
					Kv:     &mvccpb.KeyValue{Key: []byte(metadataKey), Value: []byte("new")},
					PrevKv: &mvccpb.KeyValue{Key: []byte(metadataKey), Value: []byte("old")},
				},
			},
			expected: true,
		},
		{
			name: "drain alongside an untracked key",
			events: []*clientv3.Event{
				{Type: clientv3.EventTypePut, Kv: &mvccpb.KeyValue{Key: []byte(leaderKey), Value: []byte("host-1")}},
				{Type: clientv3.EventTypePut, Kv: &mvccpb.KeyValue{Key: []byte(drainedKey)}},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, s.hasNamespaceStateChanged(clientv3.WatchResponse{Events: tt.events}, _watchTestNamespace))
		})
	}
}

func TestWatchNamespace_ReturnsWatchFailures(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := etcdclient.NewMockClient(ctrl)

	watchChan := make(chan clientv3.WatchResponse)
	mockClient.EXPECT().Watch(gomock.Any(), gomock.Any(), gomock.Any()).Return(watchChan).AnyTimes()

	s := newNamespaceWatchStore(t, mockClient, clock.NewRealTimeSource())
	changeChan := make(chan struct{}, 1)

	go func() {
		watchChan <- clientv3.WatchResponse{CompactRevision: 100}
	}()
	err := s.watchNamespace(_watchTestNamespace, changeChan)
	require.Error(t, err)
	assert.ErrorContains(t, err, "etcdserver: mvcc: required revision has been compacted")

	close(watchChan)
	err = s.watchNamespace(_watchTestNamespace, changeChan)
	require.Error(t, err)
	assert.ErrorContains(t, err, "watch channel closed")
}

// The signal channel holds one pending notification, so a subscriber that has not
// drained it must not be able to stall the watch loop.
func TestWatchNamespace_SignalsOnEstablishAndNeverBlocks(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctrl := gomock.NewController(t)
	mockClient := etcdclient.NewMockClient(ctrl)

	watchChan := make(chan clientv3.WatchResponse)
	mockClient.EXPECT().Watch(gomock.Any(), gomock.Any(), gomock.Any()).Return(watchChan).AnyTimes()

	s := newNamespaceWatchStore(t, mockClient, clock.NewRealTimeSource())
	changeChan := make(chan struct{}, 1)

	watchDone := make(chan error, 1)
	go func() {
		watchDone <- s.watchNamespace(_watchTestNamespace, changeChan)
	}()

	// A reconnected watch resumes at the current revision, so establishing one has to
	// signal on its own for the subscriber to pick up what it missed.
	select {
	case <-changeChan:
	case <-time.After(time.Second):
		t.Fatal("establishing the watch should signal the subscriber")
	}

	assignedStateKey := etcdkeys.BuildExecutorKey(_watchTestPrefix, _watchTestNamespace, _watchTestExecutor, etcdkeys.ExecutorAssignedStateKey)
	for i := 0; i < 100; i++ {
		select {
		case watchChan <- clientv3.WatchResponse{Events: []*clientv3.Event{
			{Type: clientv3.EventTypePut, Kv: &mvccpb.KeyValue{Key: []byte(assignedStateKey), Value: []byte(fmt.Sprintf("v%d", i))}},
		}}:
		case <-time.After(time.Second):
			t.Fatal("watch loop is stuck - could not send event to watchChan")
		}
	}

	close(watchChan)
	select {
	case err := <-watchDone:
		assert.ErrorContains(t, err, "watch channel closed")
	case <-time.After(time.Second):
		t.Fatal("watch loop did not exit after the watch channel closed")
	}
}

// Subscribing once the store is stopping must fail rather than hand back a channel
// that closes immediately, which a subscriber cannot tell from a quiet namespace.
func TestSubscribeToNamespaceChanges_AfterStop(t *testing.T) {
	s := newNamespaceWatchStore(t, etcdclient.NewMockClient(gomock.NewController(t)), clock.NewRealTimeSource())
	s.Stop()

	changeChan, err := s.SubscribeToNamespaceChanges(_watchTestNamespace)
	assert.Nil(t, changeChan)
	assert.ErrorContains(t, err, "store is stopping")
}

// A failed watch must be retried rather than ending the subscription, and the
// retry loop must exit once the store stops.
func TestSubscribeToNamespaceChanges_RetriesFailedWatch(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctrl := gomock.NewController(t)
	mockClient := etcdclient.NewMockClient(ctrl)
	timeSource := clock.NewMockedTimeSource()

	watchChanRcvErr := make(chan clientv3.WatchResponse)
	mockClient.EXPECT().Watch(gomock.Any(), gomock.Any(), gomock.Any()).Return(watchChanRcvErr)

	watchChanClosed := make(chan clientv3.WatchResponse)
	mockClient.EXPECT().Watch(gomock.Any(), gomock.Any(), gomock.Any()).Return(watchChanClosed)

	// A third watch may or may not be established, depending on whether the store
	// stops before the retry interval elapses.
	mockClient.EXPECT().Watch(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(make(chan clientv3.WatchResponse)).
		MinTimes(0).
		MaxTimes(1)

	s := newNamespaceWatchStore(t, mockClient, timeSource)

	changeChan, err := s.SubscribeToNamespaceChanges(_watchTestNamespace)
	require.NoError(t, err)

	closed := atomic.Bool{}
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for range changeChan {
		}
		closed.Store(true)
	}()

	// A compact revision makes WatchResponse.Err() non-nil, failing the watch.
	watchChanRcvErr <- clientv3.WatchResponse{CompactRevision: 100}
	timeSource.BlockUntil(1)
	require.False(t, closed.Load(), "a failed watch must not end the subscription")

	timeSource.Advance(2 * namespaceWatchRetryInterval)
	close(watchChanClosed)
	timeSource.BlockUntil(1)
	require.False(t, closed.Load(), "a closed watch channel must not end the subscription")

	s.Stop()
	<-drained
	assert.True(t, closed.Load(), "stopping the store must close the change channel")
}

// Stopping while the watcher sits in its retry backoff is the case that used to strand it:
// the mocked clock is never advanced again, so a backoff that ignored the watch context
// would leave the goroutine blocked for the rest of the process's life.
func TestSubscribeToNamespaceChanges_StopDuringRetryBackoff(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctrl := gomock.NewController(t)
	mockClient := etcdclient.NewMockClient(ctrl)
	timeSource := clock.NewMockedTimeSource()

	watchChan := make(chan clientv3.WatchResponse)
	mockClient.EXPECT().Watch(gomock.Any(), gomock.Any(), gomock.Any()).Return(watchChan)

	// A retry is permitted but should not happen: the store stops first.
	mockClient.EXPECT().Watch(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(make(chan clientv3.WatchResponse)).
		MinTimes(0).
		MaxTimes(1)

	s := newNamespaceWatchStore(t, mockClient, timeSource)

	_, err := s.SubscribeToNamespaceChanges(_watchTestNamespace)
	require.NoError(t, err)

	watchChan <- clientv3.WatchResponse{CompactRevision: 100}
	timeSource.BlockUntil(1)

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		s.Stop()
	}()

	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("the namespace watcher did not exit after the store stopped during watch backoff")
	}
}
