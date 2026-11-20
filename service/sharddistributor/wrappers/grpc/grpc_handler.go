package grpc

import (
	"context"

	apiv1 "github.com/uber/cadence-idl/go/proto/api/v1"
	"go.uber.org/yarpc"

	sharddistributorv1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1"
	"github.com/cadence-workflow/shard-manager/common/types/mapper/proto"
)

func (g GRPCHandler) Register(dispatcher *yarpc.Dispatcher) {
	dispatcher.Register(sharddistributorv1.BuildShardDistributorAPIYARPCProcedures(g))
}

func (g GRPCHandler) Health(ctx context.Context, _ *apiv1.HealthRequest) (*apiv1.HealthResponse, error) {
	response, err := g.h.Health(ctx)
	if err != nil {
		return nil, err
	}
	return proto.FromHealthResponse(response), proto.FromError(err)
}

func (g ExecutorGRPCExecutor) Register(dispatcher *yarpc.Dispatcher) {
	dispatcher.Register(sharddistributorv1.BuildShardDistributorExecutorAPIYARPCProcedures(g))
}
