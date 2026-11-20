package metered

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber-go/tally"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
)

func TestShardDistributorWrappers(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		clientMock := sharddistributor.NewMockClient(ctrl)

		testScope := tally.NewTestScope("", nil)
		metricsClient := metrics.NewClient(testScope, metrics.ServiceIdx(0), metrics.MigrationConfig{})

		clientMock.EXPECT().GetShardOwner(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&types.GetShardOwnerResponse{}, nil).Times(1)

		meteredClient := NewShardDistributorClient(
			clientMock,
			metricsClient)

		_, err := meteredClient.GetShardOwner(context.Background(), &types.GetShardOwnerRequest{})
		assert.NoError(t, err)
		assert.Len(t, testScope.Snapshot().Counters(), 1, "there should be a single counter registered")
		assert.Len(t, testScope.Snapshot().Timers(), 1, "there should be a single timer registered")
	})
	t.Run("error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		clientMock := sharddistributor.NewMockClient(ctrl)

		testScope := tally.NewTestScope("", nil)
		metricsClient := metrics.NewClient(testScope, metrics.ServiceIdx(0), metrics.MigrationConfig{})

		clientMock.EXPECT().GetShardOwner(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("error"))

		meteredClient := NewShardDistributorClient(
			clientMock,
			metricsClient)

		_, err := meteredClient.GetShardOwner(context.Background(), &types.GetShardOwnerRequest{})
		assert.Error(t, err)
		assert.Len(t, testScope.Snapshot().Counters(), 2, "there should be two counters registered, one for call and one for failure")
	})
}
