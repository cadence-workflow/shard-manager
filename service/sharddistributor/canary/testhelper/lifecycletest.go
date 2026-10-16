// Package testhelper provides shared test helpers for the canary subpackages.
package testhelper

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/latencykind"
	canarymetrics "github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/metrics"
)

// Lifecycler is the minimal Start/Stop surface AssertInjectsLifecycleDelay
// drives. Both canary shard processors satisfy it.
type Lifecycler interface {
	Start(context.Context) error
	Stop()
}

// LifecyclerFactory builds a Lifecycler for the shared lifecycle delay test.
type LifecyclerFactory func(shardID string, ts clock.TimeSource, logger *zap.Logger, scope tally.Scope) Lifecycler

// AssertInjectsLifecycleDelay drives the given factory through Start/Stop with
// shardIDs that hash to non-normal latency kinds and asserts the
// CanaryShardLifecycleInjected counter fires for the matching lifecycle.
func AssertInjectsLifecycleDelay(t *testing.T, factory LifecyclerFactory) {
	t.Helper()

	tests := []struct {
		name         string
		shardID      string
		kind         latencykind.Kind
		wantStartTag bool
		wantStopTag  bool
	}{
		// shardID fixtures rely on the farm.Fingerprint32 hash in
		// latencykind.ShardIDToKind — the require.Equal check below makes the
		// dependency loud if the distribution ever changes.
		{name: "slow_start", shardID: "25", kind: latencykind.SlowStart, wantStartTag: true},
		{name: "slow_stop", shardID: "11", kind: latencykind.SlowStop, wantStopTag: true},
		{name: "stuck_start", shardID: "68", kind: latencykind.StuckStart, wantStartTag: true},
		{name: "stuck_stop", shardID: "6", kind: latencykind.StuckStop, wantStopTag: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.kind, latencykind.ShardIDToKind(tt.shardID), "fixture out of sync with latencykind distribution")

			scope := tally.NewTestScope("", nil)
			tc := clock.NewMockedTimeSource()
			p := factory(tt.shardID, tc, zaptest.NewLogger(t), scope)

			startErr := make(chan error, 1)
			go func() { startErr <- p.Start(context.Background()) }()
			if d := tt.kind.StartDelay(); d > 0 {
				tc.BlockUntil(1)
				tc.Advance(d)
			}
			require.NoError(t, <-startErr)

			// Start returns before the background process loop registers its
			// ticker; calling Stop earlier lets the loop exit without ever
			// creating a sleeper, so the sleeper count below would never be reached.
			tc.BlockUntil(1)

			stopDone := make(chan struct{})
			go func() { p.Stop(); close(stopDone) }()
			if d := tt.kind.StopDelay(); d > 0 {
				// The process loop ticker plus the injected Stop sleep.
				tc.BlockUntil(2)
				tc.Advance(d)
			}
			<-stopDone

			assert.Equal(t, tt.wantStartTag, hasLifecycleInjected(scope, "start"))
			assert.Equal(t, tt.wantStopTag, hasLifecycleInjected(scope, "stop"))
		})
	}
}

func hasLifecycleInjected(scope tally.TestScope, lifecycle string) bool {
	for _, c := range scope.Snapshot().Counters() {
		if c.Name() == canarymetrics.CanaryShardLifecycleInjected && c.Tags()["lifecycle"] == lifecycle && c.Value() > 0 {
			return true
		}
	}
	return false
}
