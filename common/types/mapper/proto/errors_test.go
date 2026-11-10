package proto

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/yarpc/yarpcerrors"

	"github.com/cadence-workflow/shard-manager/common/types"
)

func TestErrors(t *testing.T) {
	testCases := []error{
		&types.InternalServiceError{Message: "internal error"},
		&types.BadRequestError{Message: "bad request"},
		&types.NamespaceNotFoundError{Namespace: "test-ns"},
		&types.ShardNotFoundError{Namespace: "test-ns", ShardKey: "shard-1"},
		&types.ShardDrainedError{Namespace: "test-ns", ShardKey: "shard-1"},
	}
	for _, err := range testCases {
		t.Run(err.Error(), func(t *testing.T) {
			assert.Equal(t, err, ToError(FromError(err)))
		})
	}
}

func TestNilMapsToOK(t *testing.T) {
	protoNoError := FromError(nil)
	assert.Equal(t, yarpcerrors.CodeOK, yarpcerrors.FromError(protoNoError).Code())
	assert.Nil(t, ToError(protoNoError))
}

func TestFromUnknownErrorMapsToUnknownError(t *testing.T) {
	err := errors.New("unknown error")
	protobufErr := FromError(err)
	assert.True(t, yarpcerrors.IsUnknown(protobufErr))

	clientErr := ToError(protobufErr)

	assert.True(t, yarpcerrors.IsUnknown(clientErr))
	assert.ErrorContains(t, clientErr, err.Error())
}

func TestFromUnimplementedMapsToItself(t *testing.T) {
	unimplemented := yarpcerrors.UnimplementedErrorf("not implemented")
	assert.True(t, yarpcerrors.IsUnimplemented(ToError(FromError(unimplemented))))
}

func TestToDeadlineExceededMapsToItself(t *testing.T) {
	timeout := yarpcerrors.DeadlineExceededErrorf("timeout")
	assert.Equal(t, timeout, ToError(timeout))
}
