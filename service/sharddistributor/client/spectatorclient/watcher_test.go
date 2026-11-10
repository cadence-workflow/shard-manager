package spectatorclient

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
)

func TestWatcherAppliesStreamedState(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctrl := gomock.NewController(t)
	client := sharddistributor.NewMockClient(ctrl)
	stream := sharddistributor.NewMockWatchNamespaceStateClient(ctrl)
	w := newTestWatcher(client)

	client.EXPECT().
		WatchNamespaceState(gomock.Any(), &types.WatchNamespaceStateRequest{Namespace: "test-ns"}).
		Return(stream, nil)
	stream.EXPECT().Recv().Return(&types.WatchNamespaceStateResponse{
		Executors:        []*types.ExecutorShardAssignment{{ExecutorID: "executor-1", AssignedShards: []*types.Shard{{ShardKey: "shard-1"}}}},
		DrainedShardKeys: []string{"shard-2"},
	}, nil)
	stream.EXPECT().Recv().DoAndReturn(func(...interface{}) (*types.WatchNamespaceStateResponse, error) {
		<-w.stream.ctx.Done()
		return nil, w.stream.ctx.Err()
	})

	w.Start(context.Background())
	defer w.Stop()

	owner, _, err := w.registry.lookup(context.Background(), "shard-1")
	require.NoError(t, err)
	assert.Equal(t, "executor-1", owner.ExecutorID)
	_, drained, _ := w.registry.lookup(context.Background(), "shard-2")
	assert.True(t, drained)
}

func TestWatcherReconnectsAfterStreamError(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctrl := gomock.NewController(t)
	client := sharddistributor.NewMockClient(ctrl)
	failingStream := sharddistributor.NewMockWatchNamespaceStateClient(ctrl)
	stream := sharddistributor.NewMockWatchNamespaceStateClient(ctrl)
	timeSource := clock.NewMockedTimeSource()
	testScope := tally.NewTestScope("", nil)
	w := newTestWatcher(client)
	w.timeSource = timeSource
	w.scope = testScope

	client.EXPECT().WatchNamespaceState(gomock.Any(), gomock.Any()).Return(failingStream, nil)
	failingStream.EXPECT().Recv().Return(nil, errors.New("network error"))
	client.EXPECT().WatchNamespaceState(gomock.Any(), gomock.Any()).Return(stream, nil)
	stream.EXPECT().Recv().Return(&types.WatchNamespaceStateResponse{}, nil)
	stream.EXPECT().Recv().DoAndReturn(func(...interface{}) (*types.WatchNamespaceStateResponse, error) {
		<-w.stream.ctx.Done()
		return nil, w.stream.ctx.Err()
	})

	w.Start(context.Background())
	defer w.Stop()

	// The failed stream's timeout timer and the retry sleep
	timeSource.BlockUntil(2)
	timeSource.Advance(2 * time.Second)
	_, _, err := w.registry.lookup(context.Background(), "shard-1")
	require.NoError(t, err)

	errorReconnects := testScope.Snapshot().Counters()["shard_distributor_spectator_stream_reconnects+reason=error"]
	require.NotNil(t, errorReconnects)
	assert.Equal(t, int64(1), errorReconnects.Value())
}

func newTestWatcher(client sharddistributor.Client) *streamWatcher {
	return &streamWatcher{
		namespace:  "test-ns",
		client:     client,
		registry:   newRegistry(),
		enabled:    func() bool { return true },
		scope:      tally.NoopScope,
		logger:     zap.NewNop(),
		timeSource: clock.NewRealTimeSource(),
	}
}
