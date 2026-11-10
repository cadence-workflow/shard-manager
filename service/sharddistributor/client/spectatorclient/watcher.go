package spectatorclient

import (
	"context"
	"sync"
	"time"

	"github.com/uber-go/tally"
	"go.uber.org/zap"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/backoff"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/clientcommon/tag"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/spectatorclient/metricsconstants"
)

// stateFn represents a state in the watch state machine.
// Each state is a function that blocks until a transition occurs
// and returns the next state function, or nil to stop.
// Note this is a recursive type definition.
type stateFn func(ctx context.Context) stateFn

const (
	streamRetryInterval    = 1 * time.Second
	streamRetryJitterCoeff = 0.1 // 10% jitter (900ms - 1100ms)
)

// streamWatcher keeps a watch stream open and applies what it delivers to the registry. It reconnects
// when the stream fails, reaches its timeout, or the poller finds it behind.
type streamWatcher struct {
	namespace string
	client    sharddistributor.Client
	registry  *registry
	// reconnect is where the poller asks for a new stream
	reconnect  <-chan struct{}
	enabled    EnabledFunc
	scope      tally.Scope
	logger     *zap.Logger
	timeSource clock.TimeSource

	cancel context.CancelFunc
	stream *spectatorStream
	stopWG sync.WaitGroup
}

func (w *streamWatcher) Start(ctx context.Context) {
	// Use context.WithoutCancel to inherit values but not cancellation from fx lifecycle ctx
	ctx, w.cancel = context.WithCancel(context.WithoutCancel(ctx))
	w.stopWG.Go(func() { w.watchLoop(ctx) })
}

func (w *streamWatcher) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.stopWG.Wait()
}

func (w *streamWatcher) watchLoop(ctx context.Context) {
	defer w.logger.Info("Shutting down, stopping watch loop", zap.String(tag.Namespace, w.namespace))
	w.logger.Info("Starting watch loop for namespace", zap.String(tag.Namespace, w.namespace))

	var state stateFn
	if w.enabled() {
		state = w.connectState
	} else {
		state = w.disabledState
	}

	for state != nil {
		state = state(ctx)
	}
}

func (w *streamWatcher) connectState(ctx context.Context) stateFn {
	defer w.logger.Info("Exiting connect state", zap.String(tag.Namespace, w.namespace))
	w.logger.Info("Starting connect state for namespace", zap.String(tag.Namespace, w.namespace))

	if !w.enabled() {
		return w.disabledState
	}

	stream, err := newSpectatorStream(ctx, w.client, w.timeSource, w.namespace)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}

		w.logger.Error("Failed to create stream, retrying", zap.Error(err), zap.String(tag.Namespace, w.namespace))
		if err := w.timeSource.SleepWithContext(ctx, backoff.JitDuration(streamRetryInterval, streamRetryJitterCoeff)); err != nil {
			return nil
		}
		return w.connectState
	}

	w.stream = stream
	w.registry.resetStreamed()
	// Drain the reconnect channel to make sure old requests are dropped.
	select {
	case <-w.reconnect:
	default:
	}

	return w.enabledState
}

func (w *streamWatcher) enabledState(ctx context.Context) stateFn {
	defer w.logger.Info("Exiting enabled state", zap.String(tag.Namespace, w.namespace))

	w.logger.Info("Starting enabled state for namespace", zap.String(tag.Namespace, w.namespace))

	// Recv blocks, so a reader forwards the stream and this loop can also wait for the poller.
	received := make(chan *types.WatchNamespaceStateResponse)
	stream := w.stream
	var recvErr error
	var reader sync.WaitGroup
	reader.Go(func() {
		// Closed after recvErr is written, so the loop can read it once received is closed.
		defer close(received)
		recvErr = forward(stream, received)
	})
	// Closing the stream ends the reader, so the wait is deferred first.
	defer reader.Wait()
	defer w.stream.Close()

	for {
		if !w.enabled() {
			return w.disabledState
		}

		select {
		case response, ok := <-received:
			if !ok {
				return w.streamEnded(ctx, recvErr)
			}
			w.handleResponse(response)

		case <-w.reconnect:
			w.streamReconnectCounter(metricsconstants.StreamReconnectReasonLag).Inc(1)
			return w.connectState
		}
	}
}

// streamEnded picks the next state once the stream has stopped delivering.
func (w *streamWatcher) streamEnded(ctx context.Context, recvErr error) stateFn {
	if ctx.Err() != nil {
		w.logger.Info("Recv interrupted by client shutdown", zap.String(tag.Namespace, w.namespace))
		return nil
	}

	if w.stream.ctx.Err() != nil {
		w.streamReconnectCounter(metricsconstants.StreamReconnectReasonTimeout).Inc(1)
	} else {
		w.streamReconnectCounter(metricsconstants.StreamReconnectReasonError).Inc(1)
		w.logger.Warn("Stream recv error, will reconnect", zap.Error(recvErr), zap.String(tag.Namespace, w.namespace))
	}

	if err := w.timeSource.SleepWithContext(ctx, backoff.JitDuration(streamRetryInterval, streamRetryJitterCoeff)); err != nil {
		return nil
	}
	return w.connectState
}

// forward sends each response on received until the stream ends and returns the error that ended it.
func forward(stream *spectatorStream, received chan<- *types.WatchNamespaceStateResponse) error {
	for {
		response, err := stream.Recv()
		if err != nil {
			return err
		}
		select {
		case received <- response:
		case <-stream.ctx.Done():
			return stream.ctx.Err()
		}
	}
}

func (w *streamWatcher) disabledState(ctx context.Context) stateFn {
	defer w.logger.Info("Exiting disabled state", zap.String(tag.Namespace, w.namespace))
	w.logger.Info("Starting disabled state for namespace", zap.String(tag.Namespace, w.namespace))
	// Reset so lookups wait for the first state again when we re-enable.
	w.registry.resetReady()

	for {
		// Sleep for a short period of time before checking again.
		// If the context is cancelled, we return nil to exit the loop.
		if err := w.timeSource.SleepWithContext(ctx, backoff.JitDuration(streamRetryInterval, streamRetryJitterCoeff)); err != nil {
			return nil
		}
		if w.enabled() {
			return w.connectState
		}
	}
}

func (w *streamWatcher) handleResponse(response *types.WatchNamespaceStateResponse) {
	if !w.registry.applyStreamed(response) {
		w.logger.Info("Discarding namespace state older than the applied state",
			zap.String(tag.Namespace, w.namespace),
			zap.Int64("assignmentRevision", response.AssignmentRevision))
		return
	}

	w.logger.Debug("Received namespace state update",
		zap.String(tag.Namespace, w.namespace),
		zap.Int(tag.Count, len(response.Executors)))
}

func (w *streamWatcher) streamReconnectCounter(reason string) tally.Counter {
	return w.scope.Tagged(map[string]string{
		metricsconstants.StreamReconnectReasonTagName: reason,
	}).Counter(metricsconstants.ShardDistributorSpectatorStreamReconnects)
}
