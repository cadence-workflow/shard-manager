// The MIT License (MIT)

// Copyright (c) 2017-2020 Uber Technologies Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

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
