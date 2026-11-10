package spectatorclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"go.uber.org/yarpc/yarpcerrors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
)

func TestPoller(t *testing.T) {
	tests := []struct {
		name               string
		streamedDuringWait int64
		wantReconnect      bool
	}{
		{name: "stream stays behind", streamedDuringWait: 4, wantReconnect: true},
		{name: "stream catches up within the threshold", streamedDuringWait: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer goleak.VerifyNone(t)
			p, client, timeSource := newTestPoller(t)
			p.registry.applyStreamed(&types.WatchNamespaceStateResponse{AssignmentRevision: 3})
			client.EXPECT().
				GetNamespaceStateIfNewer(gomock.Any(), &types.GetNamespaceStateIfNewerRequest{Namespace: "test-ns", AssignmentRevision: 3}).
				Return(&types.GetNamespaceStateIfNewerResponse{AssignmentRevision: 5, State: &types.WatchNamespaceStateResponse{AssignmentRevision: 5}}, nil)

			p.Start(context.Background())

			timeSource.BlockUntil(1)
			timeSource.Advance(2 * pollInterval) // poll, which finds the stream behind
			timeSource.BlockUntil(2)             // the next poll and the lag deadline
			p.registry.applyStreamed(&types.WatchNamespaceStateResponse{AssignmentRevision: tt.streamedDuringWait})
			timeSource.Advance(lagThreshold)
			time.Sleep(10 * time.Millisecond) // let the poller handle the deadline

			p.Stop()
			assert.Equal(t, tt.wantReconnect, len(p.reconnect) == 1)
			assert.Equal(t, int64(5), p.registry.appliedRevision(), "the polled state is applied")
		})
	}
}

func TestFailedPollIsNotLag(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantLevel zapcore.Level
	}{
		{name: "server without the endpoint", err: yarpcerrors.UnimplementedErrorf("unrecognized procedure"), wantLevel: zapcore.InfoLevel},
		{name: "real failure", err: yarpcerrors.InternalErrorf("boom"), wantLevel: zapcore.ErrorLevel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, client, _ := newTestPoller(t)
			core, logs := observer.New(zapcore.DebugLevel)
			p.logger = zap.New(core)
			client.EXPECT().GetNamespaceStateIfNewer(gomock.Any(), gomock.Any()).Return(nil, tt.err)

			_, ok := p.poll(context.Background())
			assert.False(t, ok)
			assert.Equal(t, tt.wantLevel, logs.All()[0].Level)
		})
	}
}

func newTestPoller(t *testing.T) (*poller, *sharddistributor.MockClient, clock.MockedTimeSource) {
	client := sharddistributor.NewMockClient(gomock.NewController(t))
	timeSource := clock.NewMockedTimeSource()
	return &poller{
		namespace:  "test-ns",
		client:     client,
		registry:   newRegistry(),
		timeSource: timeSource,
		logger:     zap.NewNop(),
		enabled:    func() bool { return true },
		reconnect:  make(chan struct{}, 1),
	}, client, timeSource
}
