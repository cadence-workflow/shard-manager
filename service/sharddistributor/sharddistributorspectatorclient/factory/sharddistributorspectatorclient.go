package factory

import (
	"github.com/uber-go/tally"
	"go.uber.org/fx"

	sharddistributorv1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1"
	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/client/wrappers/grpc"
	"github.com/cadence-workflow/shard-manager/client/wrappers/retryable"
	"github.com/cadence-workflow/shard-manager/client/wrappers/timeout"
	"github.com/cadence-workflow/shard-manager/common"
)

type Params struct {
	fx.In

	YarpcClient  sharddistributorv1.ShardDistributorAPIYARPCClient
	MetricsScope tally.Scope
}

func NewShardDistributorSpectatorClient(params Params) (sharddistributor.Client, error) {
	// Wrap the YARPC client with GRPC wrapper
	client := grpc.NewShardDistributorClient(params.YarpcClient)

	// Add timeout wrapper
	client = timeout.NewShardDistributorClient(client, timeout.ShardDistributorDefaultTimeout)

	// Add metered wrapper
	if params.MetricsScope != nil {
		client = NewMeteredShardDistributorClient(client, params.MetricsScope)
	}

	// Add retry wrapper
	client = retryable.NewShardDistributorClient(
		client,
		common.CreateShardDistributorServiceRetryPolicy(),
		common.IsServiceTransientError,
	)

	return client, nil
}
