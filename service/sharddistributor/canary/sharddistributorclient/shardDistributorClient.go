package sharddistributorclient

import (
	"go.uber.org/fx"

	sharddistributorv1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1"
	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/client/wrappers/grpc"
	timeoutwrapper "github.com/cadence-workflow/shard-manager/client/wrappers/timeout"
)

// Params contains the dependencies needed to create a shard distributor client
type Params struct {
	fx.In

	YarpcClient sharddistributorv1.ShardDistributorAPIYARPCClient
}

// NewShardDistributorClient creates a new shard distributor client with GRPC and timeout wrappers
func NewShardDistributorClient(p Params) (sharddistributor.Client, error) {
	shardDistributorExecutorClient := grpc.NewShardDistributorClient(p.YarpcClient)
	shardDistributorExecutorClient = timeoutwrapper.NewShardDistributorClient(shardDistributorExecutorClient, timeoutwrapper.ShardDistributorExecutorDefaultTimeout)
	return shardDistributorExecutorClient, nil
}
