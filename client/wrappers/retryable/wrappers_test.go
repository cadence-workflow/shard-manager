package retryable

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common"
	"github.com/cadence-workflow/shard-manager/common/types"
)

func TestShardDistributorClientRetryableError(t *testing.T) {
	ctrl := gomock.NewController(t)
	clientMock := sharddistributor.NewMockClient(ctrl)
	// One failure, one success
	clientMock.EXPECT().GetShardOwner(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, &types.ServiceBusyError{
			Message: "error",
		}).Times(1)
	clientMock.EXPECT().GetShardOwner(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&types.GetShardOwnerResponse{}, nil).Times(1)

	retryableClient := NewShardDistributorClient(
		clientMock,
		common.CreateShardDistributorServiceRetryPolicy(),
		common.IsServiceBusyError)

	_, err := retryableClient.GetShardOwner(context.Background(), &types.GetShardOwnerRequest{})
	assert.NoError(t, err)
}

func TestShardDistributorClientNonRetryableError(t *testing.T) {
	ctrl := gomock.NewController(t)
	clientMock := sharddistributor.NewMockClient(ctrl)
	// One failure
	clientMock.EXPECT().GetShardOwner(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, &types.BadRequestError{
			Message: "error",
		}).Times(1)

	retryableClient := NewShardDistributorClient(
		clientMock,
		common.CreateShardDistributorServiceRetryPolicy(),
		common.IsServiceBusyError)

	_, err := retryableClient.GetShardOwner(context.Background(), &types.GetShardOwnerRequest{})
	assert.Error(t, err)
}
