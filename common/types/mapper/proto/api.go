package proto

import (
	apiv1 "github.com/uber/cadence-idl/go/proto/api/v1"

	"github.com/cadence-workflow/shard-manager/common/types"
)

func FromHealthResponse(t *types.HealthStatus) *apiv1.HealthResponse {
	if t == nil {
		return nil
	}
	return &apiv1.HealthResponse{
		Ok:      t.Ok,
		Message: t.Msg,
	}
}

func ToHealthResponse(t *apiv1.HealthResponse) *types.HealthStatus {
	if t == nil {
		return nil
	}
	return &types.HealthStatus{
		Ok:  t.Ok,
		Msg: t.Message,
	}
}
