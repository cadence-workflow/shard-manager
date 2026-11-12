package hedged

import (
	"context"
	"time"

	"github.com/uber-go/tally"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/metrics"
)

const (
	// hedgeDelay avoids hedging calls that are already fast.
	hedgeDelay = 100 * time.Millisecond

	hedgesSent = "shard_distributor_client_hedges"
	hedgeWins  = "shard_distributor_client_hedge_wins"
)

type hedger struct {
	timeSource clock.TimeSource
	metrics    tally.Scope
}

type result[T any] struct {
	resp    T
	err     error
	isHedge bool
}

// hedge starts a second attempt if op is still running after hedgeDelay and returns whichever finishes first.
// It is a function rather than a hedger method because Go methods cannot have type parameters.
func hedge[T any](ctx context.Context, h hedger, operation string, op func(context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Buffered so the losing attempt never blocks after we return.
	results := make(chan result[T], 2)
	run := func(isHedge bool) {
		resp, err := op(ctx)
		results <- result[T]{resp: resp, err: err, isHedge: isHedge}
	}
	go run(false)

	timer := h.timeSource.NewTimer(hedgeDelay)
	defer timer.Stop()

	scope := h.metrics.Tagged(map[string]string{metrics.OperationTagName: operation})
	for {
		select {
		case <-timer.Chan():
			scope.Counter(hedgesSent).Inc(1)
			go run(true)
		case r := <-results:
			if r.isHedge && r.err == nil {
				scope.Counter(hedgeWins).Inc(1)
			}
			return r.resp, r.err
		}
	}
}
