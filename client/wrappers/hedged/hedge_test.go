package hedged

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"

	"github.com/cadence-workflow/shard-manager/common/clock"
)

func TestHedge(t *testing.T) {
	t.Run("fast success sends no hedge", func(t *testing.T) {
		h, _, scope := newTestHedger()

		resp, err := hedge(context.Background(), h, "op", func(context.Context) (string, error) {
			return "first", nil
		})

		require.NoError(t, err)
		assert.Equal(t, "first", resp)
		assert.Zero(t, counter(scope, hedgesSent))
	})

	t.Run("fast failure sends no hedge", func(t *testing.T) {
		h, _, scope := newTestHedger()

		_, err := hedge(context.Background(), h, "op", func(context.Context) (string, error) {
			return "", assert.AnError
		})

		assert.ErrorIs(t, err, assert.AnError)
		assert.Zero(t, counter(scope, hedgesSent))
	})

	t.Run("hedge wins and cancels the slow attempt", func(t *testing.T) {
		h, timeSource, scope := newTestHedger()
		cancelled := make(chan struct{})

		resp, err := hedge(context.Background(), h, "op", slowFirstAttempt(timeSource, cancelled, "hedge", nil))

		require.NoError(t, err)
		assert.Equal(t, "hedge", resp)
		<-cancelled
		assert.Equal(t, int64(1), counter(scope, hedgesSent))
		assert.Equal(t, int64(1), counter(scope, hedgeWins))
	})

	t.Run("hedge error is returned and cancels the slow attempt", func(t *testing.T) {
		h, timeSource, scope := newTestHedger()
		cancelled := make(chan struct{})

		_, err := hedge(context.Background(), h, "op", slowFirstAttempt(timeSource, cancelled, "", assert.AnError))

		assert.ErrorIs(t, err, assert.AnError)
		<-cancelled
		assert.Equal(t, int64(1), counter(scope, hedgesSent))
		assert.Zero(t, counter(scope, hedgeWins))
	})

	t.Run("original wins and cancels the slow hedge", func(t *testing.T) {
		h, timeSource, scope := newTestHedger()
		hedgeStarted := make(chan struct{})
		cancelled := make(chan struct{})
		var calls atomic.Int32

		resp, err := hedge(context.Background(), h, "op", func(ctx context.Context) (string, error) {
			if calls.Add(1) > 1 {
				defer close(cancelled)
				close(hedgeStarted)
				<-ctx.Done()
				return "", ctx.Err()
			}
			timeSource.BlockUntil(1)
			timeSource.Advance(hedgeDelay)
			<-hedgeStarted
			return "first", nil
		})

		require.NoError(t, err)
		assert.Equal(t, "first", resp)
		<-cancelled
		assert.Equal(t, int64(1), counter(scope, hedgesSent))
		assert.Zero(t, counter(scope, hedgeWins))
	})
}

func newTestHedger() (hedger, clock.MockedTimeSource, tally.TestScope) {
	timeSource := clock.NewMockedTimeSource()
	scope := tally.NewTestScope("", nil)
	return hedger{timeSource: timeSource, metrics: scope}, timeSource, scope
}

// slowFirstAttempt returns an op whose first call fires the hedge timer and then waits to be cancelled,
// closing cancelled when it returns. The second call returns resp and err.
func slowFirstAttempt(timeSource clock.MockedTimeSource, cancelled chan struct{}, resp string, err error) func(context.Context) (string, error) {
	var calls atomic.Int32
	return func(ctx context.Context) (string, error) {
		if calls.Add(1) > 1 {
			return resp, err
		}
		defer close(cancelled)
		timeSource.BlockUntil(1)
		timeSource.Advance(hedgeDelay)
		<-ctx.Done()
		return "", ctx.Err()
	}
}

func counter(scope tally.TestScope, name string) int64 {
	var total int64
	for _, c := range scope.Snapshot().Counters() {
		if c.Name() == name {
			total += c.Value()
		}
	}
	return total
}
