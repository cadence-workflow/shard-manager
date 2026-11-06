package spectatorclient

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/uber-go/tally"
	"go.uber.org/zap"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/backoff"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/clientcommon"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/clientcommon/tag"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/spectatorclient/metricsconstants"
	csync "github.com/cadence-workflow/shard-manager/service/sharddistributor/client/spectatorclient/sync"
)

// stateFn represents a state in the election state machine.
// Each state is a function that blocks until a transition occurs
// and returns the next state function, or nil to stop.
// Note this is a recursive type definition.
type stateFn func(ctx context.Context) stateFn

const (
	streamRetryInterval    = 1 * time.Second
	streamRetryJitterCoeff = 0.1 // 10% jitter (900ms - 1100ms)

	lagCheckInterval    = 10 * time.Second
	lagCheckJitterCoeff = 0.2
	lagCheckTimeout     = 2 * time.Second
)

// ShardOwner contains information about the executor that owns a shard
type ShardOwner struct {
	ExecutorID string
	Metadata   map[string]string
}

type spectatorImpl struct {
	namespace  string
	enabled    EnabledFunc
	config     clientcommon.NamespaceConfig
	client     sharddistributor.Client
	scope      tally.Scope
	logger     *zap.Logger
	timeSource clock.TimeSource

	cancel context.CancelFunc
	stream *spectatorStream
	stopWG sync.WaitGroup

	// State storage with lock for thread-safe access
	// Map from shard ID to shard owner (executor ID + metadata)
	stateMu      sync.RWMutex
	shardToOwner map[string]*ShardOwner
	// drainedShards is the local drain set
	drainedShards map[string]struct{}
	// appliedRevision is the assignment revision of the applied state, which the stream or a poll may have delivered.
	appliedRevision int64

	// Signal to notify when first state is received
	firstStateSignal *csync.ResettableSignal
}

func (s *spectatorImpl) Start(ctx context.Context) error {
	// Create a cancellable context for the lifetime of the spectator
	// Use context.WithoutCancel to inherit values but not cancellation from fx lifecycle ctx
	ctx, s.cancel = context.WithCancel(context.WithoutCancel(ctx))

	s.stopWG.Add(1)
	go func() {
		defer s.stopWG.Done()
		s.watchLoop(ctx)
	}()

	return nil
}

func (s *spectatorImpl) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	// Close the firstStateSignal to unblock any goroutines waiting for first state
	s.firstStateSignal.Done()
	s.stopWG.Wait()
}

func (s *spectatorImpl) watchLoop(ctx context.Context) {
	defer s.logger.Info("Shutting down, stopping watch loop", zap.String(tag.Namespace, s.namespace))
	s.logger.Info("Starting watch loop for namespace", zap.String(tag.Namespace, s.namespace))

	var state stateFn
	if s.enabled() {
		state = s.connectState
	} else {
		state = s.disabledState
	}

	for state != nil {
		state = state(ctx)
	}
}

func (s *spectatorImpl) connectState(ctx context.Context) stateFn {
	defer s.logger.Info("Exiting connect state", zap.String(tag.Namespace, s.namespace))
	s.logger.Info("Starting connect state for namespace", zap.String(tag.Namespace, s.namespace))

	if !s.enabled() {
		return s.disabledState
	}

	stream, err := newSpectatorStream(ctx, s.client, s.timeSource, s.namespace)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}

		s.logger.Error("Failed to create stream, retrying", zap.Error(err), zap.String(tag.Namespace, s.namespace))
		if err := s.timeSource.SleepWithContext(ctx, backoff.JitDuration(streamRetryInterval, streamRetryJitterCoeff)); err != nil {
			return nil
		}
		return s.connectState
	}

	s.stream = stream

	return s.enabledState
}

func (s *spectatorImpl) enabledState(ctx context.Context) stateFn {
	defer s.logger.Info("Exiting enabled state", zap.String(tag.Namespace, s.namespace))
	// Closing the stream ends the reader, so the wait is deferred first.
	var reader sync.WaitGroup
	defer reader.Wait()
	defer s.stream.Close()

	s.logger.Info("Starting enabled state for namespace", zap.String(tag.Namespace, s.namespace))

	// Recv blocks, so a reader forwards the stream and the watch loop owns all the lag check state.
	received := make(chan *types.WatchNamespaceStateResponse)
	stream := s.stream
	reader.Go(func() { stream.forward(received) })

	return s.watch(ctx, received)
}

// watch applies the stream responses and polls for lag until the stream ends, then returns the next state.
func (s *spectatorImpl) watch(ctx context.Context, received <-chan *types.WatchNamespaceStateResponse) stateFn {
	streamed := int64(-1)            // assignment revision the stream last delivered, -1 until it delivers
	var lagging int64                // polled assignment revision the stream had not delivered
	var lagDeadline <-chan time.Time // nil unless the stream was behind at the last poll
	pollTimer := s.timeSource.NewTimer(backoff.JitDuration(lagCheckInterval, lagCheckJitterCoeff))
	defer pollTimer.Stop()

	for {
		if !s.enabled() {
			return s.disabledState
		}

		select {
		case response, ok := <-received:
			if !ok {
				if ctx.Err() != nil {
					s.logger.Info("Recv interrupted by client shutdown", zap.String(tag.Namespace, s.namespace))
					return nil
				}

				if s.stream.ctx.Err() != nil {
					s.streamReconnectCounter(metricsconstants.StreamReconnectReasonTimeout).Inc(1)
				} else {
					s.streamReconnectCounter(metricsconstants.StreamReconnectReasonError).Inc(1)
					s.logger.Warn("Stream recv error, will reconnect", zap.Error(s.stream.recvErr), zap.String(tag.Namespace, s.namespace))
				}

				if err := s.timeSource.SleepWithContext(ctx, backoff.JitDuration(streamRetryInterval, streamRetryJitterCoeff)); err != nil {
					return nil
				}
				return s.connectState
			}

			// Process the response
			streamed = response.AssignmentRevision
			s.handleResponse(response)

		case <-pollTimer.Chan():
			if polled, ok := s.poll(ctx); ok && streamBehind(streamed, polled) {
				// The threshold absorbs a push still in flight when the poll was answered.
				lagging = polled
				lagDeadline = s.timeSource.After(s.config.GetStreamLagThreshold())
				continue
			}
			pollTimer.Reset(backoff.JitDuration(lagCheckInterval, lagCheckJitterCoeff))

		case <-lagDeadline:
			if streamBehind(streamed, lagging) {
				s.logger.Warn("Stream is behind the shard distributor, reconnecting",
					zap.String(tag.Namespace, s.namespace),
					zap.Int64("polledAssignmentRevision", lagging))
				s.streamReconnectCounter(metricsconstants.StreamReconnectReasonLag).Inc(1)
				return s.connectState
			}
			lagDeadline = nil
			pollTimer.Reset(backoff.JitDuration(lagCheckInterval, lagCheckJitterCoeff))
		}
	}
}

func (s *spectatorImpl) disabledState(ctx context.Context) stateFn {
	if ctx.Err() != nil {
		// shutting down — Reset() would replace doneCh with a new channel
		// nobody will close, causing Wait() callers to block forever
		return nil
	}
	defer s.logger.Info("Exiting disabled state", zap.String(tag.Namespace, s.namespace))
	s.logger.Info("Starting disabled state for namespace", zap.String(tag.Namespace, s.namespace))
	// We reset the first state signal to ensure we wait for the first state to be received when we re-enable.
	s.firstStateSignal.Reset()

	for {
		// Sleep for a short period of time before checking again.
		// If the context is cancelled, we return nil to exit the loop.
		if err := s.timeSource.SleepWithContext(ctx, backoff.JitDuration(streamRetryInterval, streamRetryJitterCoeff)); err != nil {
			return nil
		}
		if s.enabled() {
			return s.connectState
		}
	}
}

func (s *spectatorImpl) handleResponse(response *types.WatchNamespaceStateResponse) {
	// Build inverted map: shard ID -> shard owner (executor ID + metadata)
	shardToOwner := make(map[string]*ShardOwner)
	for _, executor := range response.Executors {
		owner := &ShardOwner{
			ExecutorID: executor.ExecutorID,
			Metadata:   executor.Metadata,
		}
		for _, shard := range executor.AssignedShards {
			shardToOwner[shard.ShardKey] = owner
		}
	}

	drainedShards := make(map[string]struct{}, len(response.DrainedShardKeys))
	for _, shardKey := range response.DrainedShardKeys {
		drainedShards[shardKey] = struct{}{}
	}

	s.stateMu.Lock()
	// Older servers send no revision, so their state is always applied.
	if response.AssignmentRevision != 0 && response.AssignmentRevision < s.appliedRevision {
		s.stateMu.Unlock()
		s.logger.Info("Discarding namespace state older than the applied state",
			zap.String(tag.Namespace, s.namespace),
			zap.Int64("assignmentRevision", response.AssignmentRevision),
			zap.Int64("appliedRevision", s.appliedRevision))
		return
	}
	s.shardToOwner = shardToOwner
	s.drainedShards = drainedShards
	s.appliedRevision = response.AssignmentRevision
	s.stateMu.Unlock()

	// Signal that first state has been received - this function is free to call
	// multiple times.
	s.firstStateSignal.Done()

	s.logger.Debug("Received namespace state update",
		zap.String(tag.Namespace, s.namespace),
		zap.Int(tag.Count, len(response.Executors)))
}

func (s *spectatorImpl) poll(ctx context.Context) (int64, bool) {
	ctx, cancel := context.WithTimeout(ctx, lagCheckTimeout)
	defer cancel()
	response, err := s.client.GetNamespaceStateIfNewer(ctx, &types.GetNamespaceStateIfNewerRequest{
		Namespace:          s.namespace,
		AssignmentRevision: s.appliedRevision,
	})
	if err != nil {
		s.logger.Error("Lag check poll failed", zap.Error(err), zap.String(tag.Namespace, s.namespace))
		return 0, false
	}

	if response.State != nil {
		s.handleResponse(response.State)
	}
	return response.AssignmentRevision, true
}

// streamBehind reports whether the stream has not delivered the polled assignment revision.
// State from an older server carries no revision and cannot be compared.
func streamBehind(streamed, polled int64) bool {
	return streamed != 0 && streamed < polled
}

// GetShardOwner returns the full owner information including metadata for a given shard.
// It first waits for the initial state to be received, then checks the cache.
// If not found in cache, it falls back to querying the shard distributor directly.
//
// A drained shard is refused from the local state without an RPC. Drain is checked
// before ownership so the answer matches the shard distributor, which also refuses a
// drained shard while it still has an owner.
func (s *spectatorImpl) GetShardOwner(ctx context.Context, shardKey string) (*ShardOwner, error) {
	// Wait for first state to be received to avoid flooding shard distributor on startup
	if err := s.firstStateSignal.Wait(ctx); err != nil {
		return nil, fmt.Errorf("wait for first state: %w", err)
	}

	// Check cache first
	s.stateMu.RLock()
	_, drained := s.drainedShards[shardKey]
	owner := s.shardToOwner[shardKey]
	s.stateMu.RUnlock()

	if drained {
		return nil, &types.ShardDrainedError{
			Namespace: s.namespace,
			ShardKey:  shardKey,
		}
	}

	if owner != nil {
		return owner, nil
	}

	// Cache miss - fall back to RPC call
	s.logger.Debug("Shard not found in cache, querying shard distributor",
		zap.String(tag.ShardKey, shardKey),
		zap.String(tag.Namespace, s.namespace))

	response, err := s.client.GetShardOwner(ctx, &types.GetShardOwnerRequest{
		Namespace: s.namespace,
		ShardKey:  shardKey,
	})
	if err != nil {
		return nil, fmt.Errorf("get shard owner from shard distributor: %w", err)
	}

	return &ShardOwner{
		ExecutorID: response.Owner,
		Metadata:   response.Metadata,
	}, nil
}

func (s *spectatorImpl) streamReconnectCounter(reason string) tally.Counter {
	return s.scope.Tagged(map[string]string{
		metricsconstants.StreamReconnectReasonTagName: reason,
	}).Counter(metricsconstants.ShardDistributorSpectatorStreamReconnects)
}
