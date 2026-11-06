package spectatorclient

import (
	"context"
	"time"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
)

const defaultStreamTimeout = 10 * time.Minute

// spectatorStream holds the state for an active GRPC stream.
// A timer cancels the stream context after a timeout to force reconnection,
// avoiding network infrastructure silently killing long-lived connections.
type spectatorStream struct {
	sharddistributor.WatchNamespaceStateClient
	ctx    context.Context
	cancel context.CancelFunc
	timer  clock.Timer
	// recvErr is the error that ended the stream, written before forward closes its channel
	recvErr error
}

func newSpectatorStream(
	ctx context.Context,
	client sharddistributor.Client,
	timeSource clock.TimeSource,
	namespace string,
) (*spectatorStream, error) {
	streamCtx, streamCancel := context.WithCancel(ctx)
	timer := timeSource.AfterFunc(defaultStreamTimeout, streamCancel)

	stream, err := client.WatchNamespaceState(streamCtx, &types.WatchNamespaceStateRequest{
		Namespace: namespace,
	})
	if err != nil {
		timer.Stop()
		streamCancel()
		return nil, err
	}

	return &spectatorStream{
		WatchNamespaceStateClient: stream,
		ctx:                       streamCtx,
		cancel:                    streamCancel,
		timer:                     timer,
	}, nil
}

func (ss *spectatorStream) Close() {
	ss.timer.Stop()
	ss.cancel()
}

// forward sends each response on received until the stream ends, then closes received.
// Closing is a signal the receiver cannot miss, even when the end races a send.
func (ss *spectatorStream) forward(received chan<- *types.WatchNamespaceStateResponse) {
	defer close(received)
	for {
		response, err := ss.Recv()
		if err != nil {
			ss.recvErr = err
			return
		}
		select {
		case received <- response:
		case <-ss.ctx.Done():
			return
		}
	}
}
