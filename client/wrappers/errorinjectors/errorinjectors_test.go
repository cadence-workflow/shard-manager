package errorinjectors

import (
	"context"
	stdErrors "errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/errors"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/types"
)

func TestShardDistributorInjector(t *testing.T) {
	t.Run("no error is forwarded", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		clientMock := sharddistributor.NewMockClient(ctrl)

		clientMock.EXPECT().GetShardOwner(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)

		injector := NewShardDistributorClient(clientMock, 1, testlogger.New(t)).(*sharddistributorClient)

		injector.fakeErrFn = func(float64) error {
			return nil
		}
		injector.forwardCallFn = func(err error) bool {
			return err == nil
		}
		_, err := injector.GetShardOwner(context.Background(), &types.GetShardOwnerRequest{})
		assert.NoError(t, err)
	})
	t.Run("no fake error, but client returns an error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		clientMock := sharddistributor.NewMockClient(ctrl)

		clientErr := fmt.Errorf("fail")

		clientMock.EXPECT().GetShardOwner(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, clientErr).Times(1)

		injector := NewShardDistributorClient(clientMock, 1, testlogger.New(t)).(*sharddistributorClient)

		injector.fakeErrFn = func(float64) error {
			return nil
		}
		injector.forwardCallFn = func(err error) bool {
			return err == nil
		}
		_, err := injector.GetShardOwner(context.Background(), &types.GetShardOwnerRequest{})
		assert.Error(t, err)
		assert.True(t, stdErrors.Is(clientErr, err))
	})
	t.Run("fake error overrides client error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		clientMock := sharddistributor.NewMockClient(ctrl)

		clientErr := fmt.Errorf("fail")

		clientMock.EXPECT().GetShardOwner(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, clientErr).Times(1)

		injector := NewShardDistributorClient(clientMock, 1, testlogger.New(t)).(*sharddistributorClient)

		injector.fakeErrFn = func(float64) error {
			return errors.ErrFakeServiceBusy
		}
		injector.forwardCallFn = func(err error) bool {
			return true
		}
		_, err := injector.GetShardOwner(context.Background(), &types.GetShardOwnerRequest{})
		assert.Error(t, err)
		assert.True(t, stdErrors.Is(errors.ErrFakeServiceBusy, err))
	})
	t.Run("failed but not forwarded", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		clientMock := sharddistributor.NewMockClient(ctrl)

		clientMock.EXPECT().GetShardOwner(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)

		injector := NewShardDistributorClient(clientMock, 1, testlogger.New(t)).(*sharddistributorClient)

		injector.fakeErrFn = func(float64) error {
			return errors.ErrFakeServiceBusy
		}
		injector.forwardCallFn = func(err error) bool {
			return true
		}
		_, err := injector.GetShardOwner(context.Background(), &types.GetShardOwnerRequest{})
		assert.Error(t, err)
	})
	t.Run("failed and not forwarded", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		clientMock := sharddistributor.NewMockClient(ctrl)

		injector := NewShardDistributorClient(clientMock, 1, testlogger.New(t)).(*sharddistributorClient)

		injector.fakeErrFn = func(float64) error {
			return errors.ErrFakeServiceBusy
		}
		injector.forwardCallFn = func(err error) bool {
			return false
		}
		_, err := injector.GetShardOwner(context.Background(), &types.GetShardOwnerRequest{})
		assert.Error(t, err)
	})
}
