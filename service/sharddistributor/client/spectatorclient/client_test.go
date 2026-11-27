package spectatorclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber-go/tally"
	"go.uber.org/mock/gomock"
	"go.uber.org/yarpc"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
)

func TestNewClient(t *testing.T) {
	for _, share := range []float64{0.1, 0} {
		raw := sharddistributor.NewMockClient(gomock.NewController(t))
		timeSource := clock.NewMockedTimeSource()
		scope := tally.NewTestScope("", nil)
		client := newClient(raw, timeSource, scope, func() float64 { return share })
		resp := &types.GetNamespaceStateIfNewerResponse{AssignmentRevision: 7}
		// The first attempt outlasts the hedge delay, then waits for the hedge to win unless hedging is off.
		raw.EXPECT().GetNamespaceStateIfNewer(gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, _ *types.GetNamespaceStateIfNewerRequest, _ ...yarpc.CallOption) (*types.GetNamespaceStateIfNewerResponse, error) {
				timeSource.BlockUntil(1)
				timeSource.Advance(100 * time.Millisecond)
				if share == 0 {
					return resp, nil
				}
				<-ctx.Done()
				return nil, ctx.Err()
			})
		raw.EXPECT().GetNamespaceStateIfNewer(gomock.Any(), gomock.Any()).Return(resp, nil).MaxTimes(1)

		got, err := client.GetNamespaceStateIfNewer(context.Background(), &types.GetNamespaceStateIfNewerRequest{})
		assert.NoError(t, err)
		assert.Equal(t, resp, got)
		hedges := scope.Snapshot().Counters()["shard_distributor_client_hedges+operation=ShardDistributorSpectatorGetNamespaceStateIfNewer"]
		assert.Equal(t, share > 0, hedges != nil && hedges.Value() == 1, "share %v: one hedge, tagged with the operation, only when enabled", share)
	}
}
