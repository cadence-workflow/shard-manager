package spectatorclient

import (
	"context"
	"sync"
	"time"

	"go.uber.org/yarpc/yarpcerrors"
	"go.uber.org/zap"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/backoff"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/clientcommon/tag"
)

const (
	pollInterval    = 10 * time.Second
	pollJitterCoeff = 0.2
	pollTimeout     = 2 * time.Second
	lagThreshold    = 2 * time.Second
)

// poller polls periodically, it will asks the stream to reconnect if it has not delivered
// the polled revision within the lag threshold.
// Streams are tied to a single host, the polls will go to a random host (chosen by the load balancer)
// so if the host the stream is tied to is lagging, the poller will detect it and ask the stream to reconnect.
type poller struct {
	namespace  string
	client     sharddistributor.Client
	registry   *registry
	timeSource clock.TimeSource
	logger     *zap.Logger
	enabled    EnabledFunc
	reconnect  chan<- struct{}

	cancel context.CancelFunc
	stopWG sync.WaitGroup
}

func (p *poller) Start(ctx context.Context) {
	// Use context.WithoutCancel to inherit values but not cancellation from fx lifecycle ctx
	ctx, p.cancel = context.WithCancel(context.WithoutCancel(ctx))
	p.stopWG.Go(func() { p.run(ctx) })
}

func (p *poller) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.stopWG.Wait()
}

func (p *poller) run(ctx context.Context) {
	defer p.logger.Info("Shutting down, stopping poller", zap.String(tag.Namespace, p.namespace))
	p.logger.Info("Starting poller for namespace", zap.String(tag.Namespace, p.namespace))

	pollTimer := p.timeSource.NewTimer(backoff.JitDuration(pollInterval, pollJitterCoeff))
	defer pollTimer.Stop()
	var lagging int64                // polled assignment revision the stream had not delivered
	var lagDeadline <-chan time.Time // nil unless a poll found the stream behind

	for {
		select {
		case <-ctx.Done():
			return

		case <-pollTimer.Chan():
			pollTimer.Reset(backoff.JitDuration(pollInterval, pollJitterCoeff))
			if !p.enabled() {
				continue
			}
			polled, ok := p.poll(ctx)
			// If the stream is behind, and we are not already detecting it, set the lag deadline
			if ok && lagDeadline == nil && p.registry.streamBehind(polled) {
				lagging = polled
				lagDeadline = p.timeSource.After(lagThreshold)
			}

		case <-lagDeadline:
			lagDeadline = nil
			// If the stream is still behind, ask it to reconnect (this avoids reconnecting due to races between poller and stream)
			if p.registry.streamBehind(lagging) {
				p.logger.Warn("Stream is behind the shard distributor, reconnecting",
					zap.String(tag.Namespace, p.namespace),
					zap.Int64("polledAssignmentRevision", lagging))
				// If a reconnect is already pending, this one will be ignored.
				select {
				case p.reconnect <- struct{}{}:
				default:
				}
			}
		}
	}
}

func (p *poller) poll(ctx context.Context) (int64, bool) {
	ctx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()
	response, err := p.client.GetNamespaceStateIfNewer(ctx, &types.GetNamespaceStateIfNewerRequest{
		Namespace:          p.namespace,
		AssignmentRevision: p.registry.appliedRevision(),
	})
	if err != nil {
		// Servers without the endpoint answer Unimplemented, which is expected during a rollout.
		if yarpcerrors.IsUnimplemented(err) {
			p.logger.Info("Lag check poll unavailable", zap.Error(err), zap.String(tag.Namespace, p.namespace))
			return 0, false
		}
		// The client has retries built in, so there is not much we can do except to log the error and try again
		// in the next poll.
		p.logger.Error("Lag check poll failed", zap.Error(err), zap.String(tag.Namespace, p.namespace))
		return 0, false
	}

	// If we get a response, the version we knew was older than the server, so we apply the new state.
	// apply does the final revision check.
	if response.State != nil {
		p.registry.applyPolled(response.State)
	}
	return response.AssignmentRevision, true
}
