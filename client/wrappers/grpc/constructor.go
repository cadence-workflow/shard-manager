package grpc

import (
	sharddistributorv1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1"
	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/client/sharddistributorexecutor"
)

type (
	sharddistributorClient struct {
		c sharddistributorv1.ShardDistributorAPIYARPCClient
	}
	sharddistributorexecutorClient struct {
		c sharddistributorv1.ShardDistributorExecutorAPIYARPCClient
	}
)

func NewShardDistributorClient(c sharddistributorv1.ShardDistributorAPIYARPCClient) sharddistributor.Client {
	return sharddistributorClient{c}
}

func NewShardDistributorExecutorClient(c sharddistributorv1.ShardDistributorExecutorAPIYARPCClient) sharddistributorexecutor.Client {
	return sharddistributorexecutorClient{c}
}
