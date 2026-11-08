package load

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber-go/tally"
	"go.uber.org/mock/gomock"
	"go.uber.org/yarpc"
	"go.uber.org/zap/zaptest"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
	canarymetrics "github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/metrics"
)

func TestConvergenceCheckerAwait(t *testing.T) {
	tests := []struct {
		name string
		// owners returns the cached owner of shard "1" on the given lookup, starting at 1.
		owners  func(lookup int) (string, error)
		cancel  bool
		wantErr error
		// wantLookups is the exact number of shard "1" lookups, unchecked when zero.
		wantLookups      int
		wantConverged    bool
		wantTimeoutCount int64
	}{
		{
			name:          "cache already agrees",
			owners:        func(int) (string, error) { return "executor-a", nil },
			wantLookups:   1,
			wantConverged: true,
		},
		{
			name: "cache catches up",
			owners: func(lookup int) (string, error) {
				if lookup < 3 {
					return "executor-old", nil
				}
				return "executor-a", nil
			},
			wantLookups:   3,
			wantConverged: true,
		},
		{
			name:             "stale cache times out",
			owners:           func(int) (string, error) { return "executor-old", nil },
			wantTimeoutCount: 1,
		},
		{
			name:             "lookup errors count as mismatches",
			owners:           func(int) (string, error) { return "", errors.New("unavailable") },
			wantTimeoutCount: 1,
		},
		{
			name:        "shutdown stops waiting",
			owners:      func(int) (string, error) { return "executor-old", nil },
			cancel:      true,
			wantErr:     context.Canceled,
			wantLookups: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			timeSource := clock.NewMockedTimeSource()
			scope := tally.NewTestScope("", nil)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			client := sharddistributor.NewMockClient(ctrl)
			// Shard "0" agrees on the first poll and must not be looked up again.
			client.EXPECT().GetShardOwner(gomock.Any(), &types.GetShardOwnerRequest{Namespace: testNamespace, ShardKey: "0"}).
				Return(&types.GetShardOwnerResponse{Owner: "executor-b"}, nil)
			lookups := 0
			lookup := client.EXPECT().GetShardOwner(gomock.Any(), &types.GetShardOwnerRequest{Namespace: testNamespace, ShardKey: "1"}).DoAndReturn(
				func(context.Context, *types.GetShardOwnerRequest, ...yarpc.CallOption) (*types.GetShardOwnerResponse, error) {
					lookups++
					if tt.cancel {
						cancel()
					}
					owner, err := tt.owners(lookups)
					if err != nil {
						return nil, err
					}
					return &types.GetShardOwnerResponse{Owner: owner}, nil
				},
			).MinTimes(1)
			if tt.wantLookups > 0 {
				lookup.Times(tt.wantLookups)
			}

			checker := &convergenceChecker{
				client:    client,
				namespace: testNamespace,
				holders: func() []shardHolder {
					return []shardHolder{{shardKey: "0", executorID: "executor-b"}, {shardKey: "1", executorID: "executor-a"}}
				},
				timeout:    10 * time.Second,
				logger:     zaptest.NewLogger(t),
				timeSource: timeSource,
				scope:      scope,
			}

			var err error
			runUntilDone(timeSource, func() { err = checker.await(ctx, phaseScaleUp) })

			assert.ErrorIs(t, err, tt.wantErr)
			snapshot := scope.Snapshot()
			tags := "+phase=" + phaseScaleUp
			_, converged := snapshot.Timers()[canarymetrics.CanaryLoadConvergenceLatency+tags]
			assert.Equal(t, tt.wantConverged, converged)
			var timeouts int64
			if counter, ok := snapshot.Counters()[canarymetrics.CanaryLoadConvergenceTimeout+tags]; ok {
				timeouts = counter.Value()
			}
			assert.Equal(t, tt.wantTimeoutCount, timeouts)
		})
	}
}
