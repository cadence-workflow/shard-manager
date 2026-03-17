// Copyright (c) 2021 Uber Technologies Inc.
//
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

package proto

import (
	"go.uber.org/yarpc/encoding/protobuf"
	"go.uber.org/yarpc/yarpcerrors"

	sharddistributorv1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/common/types/mapper/errorutils"
)

func FromError(err error) error {
	if err == nil {
		return protobuf.NewError(yarpcerrors.CodeOK, "")
	}

	var (
		ok       bool
		typedErr error
	)
	if ok, typedErr = errorutils.ConvertError(err, fromInternalServiceError); ok {
		return typedErr
	} else if ok, typedErr = errorutils.ConvertError(err, fromBadRequestError); ok {
		return typedErr
	} else if ok, typedErr = errorutils.ConvertError(err, fromNamespaceNotFoundErr); ok {
		return typedErr
	} else if ok, typedErr = errorutils.ConvertError(err, fromShardNotFoundErr); ok {
		return typedErr
	}

	return protobuf.NewError(yarpcerrors.CodeUnknown, err.Error())
}

func ToError(err error) error {
	status := yarpcerrors.FromError(err)
	if status == nil || status.Code() == yarpcerrors.CodeOK {
		return nil
	}

	switch status.Code() {
	case yarpcerrors.CodeInternal:
		return &types.InternalServiceError{
			Message: status.Message(),
		}
	case yarpcerrors.CodeNotFound:
		switch details := getErrorDetails(err).(type) {
		case *sharddistributorv1.NamespaceNotFoundError:
			if details != nil {
				return &types.NamespaceNotFoundError{
					Namespace: details.Namespace,
				}
			}
		case *sharddistributorv1.ShardNotFoundError:
			if details != nil {
				return &types.ShardNotFoundError{
					Namespace: details.Namespace,
					ShardKey:  details.ShardKey,
				}
			}
		}
	case yarpcerrors.CodeInvalidArgument:
		return &types.BadRequestError{
			Message: status.Message(),
		}
	}

	// If error does not match anything, return raw yarpc status error
	return status
}

func getErrorDetails(err error) interface{} {
	details := protobuf.GetErrorDetails(err)
	if len(details) > 0 {
		return details[0]
	}
	return nil
}

func fromInternalServiceError(e *types.InternalServiceError) error {
	return protobuf.NewError(yarpcerrors.CodeInternal, e.Message)
}

func fromBadRequestError(e *types.BadRequestError) error {
	return protobuf.NewError(yarpcerrors.CodeInvalidArgument, e.Message)
}

func fromNamespaceNotFoundErr(e *types.NamespaceNotFoundError) error {
	return protobuf.NewError(yarpcerrors.CodeNotFound, e.Error(), protobuf.WithErrorDetails(&sharddistributorv1.NamespaceNotFoundError{
		Namespace: e.Namespace,
	}))
}

func fromShardNotFoundErr(e *types.ShardNotFoundError) error {
	return protobuf.NewError(yarpcerrors.CodeNotFound, e.Error(), protobuf.WithErrorDetails(&sharddistributorv1.ShardNotFoundError{
		Namespace: e.Namespace,
		ShardKey:  e.ShardKey,
	}))
}
