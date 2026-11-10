package proto

import (
	"go.uber.org/yarpc/encoding/protobuf"
	"go.uber.org/yarpc/yarpcerrors"

	sharddistributorv1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1"
	"github.com/cadence-workflow/shard-manager/common/types"
)

func FromError(err error) error {
	if err == nil {
		return protobuf.NewError(yarpcerrors.CodeOK, "")
	}

	switch e := err.(type) {
	case *types.InternalServiceError:
		return fromInternalServiceError(e)
	case *types.BadRequestError:
		return fromBadRequestError(e)
	case *types.NamespaceNotFoundError:
		return fromNamespaceNotFoundErr(e)
	case *types.ShardNotFoundError:
		return fromShardNotFoundErr(e)
	case *types.ShardDrainedError:
		return fromShardDrainedErr(e)
	}
	// A yarpc status already carries its code, such as Unimplemented.
	if yarpcerrors.IsStatus(err) {
		return err
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
	case yarpcerrors.CodeFailedPrecondition:
		if details, ok := getErrorDetails(err).(*sharddistributorv1.ShardDrainedError); ok && details != nil {
			return &types.ShardDrainedError{
				Namespace: details.Namespace,
				ShardKey:  details.ShardKey,
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

func fromShardDrainedErr(e *types.ShardDrainedError) error {
	return protobuf.NewError(yarpcerrors.CodeFailedPrecondition, e.Error(), protobuf.WithErrorDetails(&sharddistributorv1.ShardDrainedError{
		Namespace: e.Namespace,
		ShardKey:  e.ShardKey,
	}))
}
