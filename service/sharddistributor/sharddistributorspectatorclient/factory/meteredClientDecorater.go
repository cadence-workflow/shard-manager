package factory

import (
	"context"

	"github.com/uber-go/tally"
	"go.uber.org/yarpc"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/spectatorclient/metricsconstants"
)

// TODO: consider using gowrap to generate this code
type meteredShardDistributorClient struct {
	client       sharddistributor.Client
	metricsScope tally.Scope
}

// NewMeteredShardDistributorClient creates a new instance of metered shard distributor client
func NewMeteredShardDistributorClient(client sharddistributor.Client, metricsScope tally.Scope) sharddistributor.Client {
	return &meteredShardDistributorClient{
		client:       client,
		metricsScope: metricsScope,
	}
}

func (c *meteredShardDistributorClient) GetShardOwner(ctx context.Context, request *types.GetShardOwnerRequest, opts ...yarpc.CallOption) (*types.GetShardOwnerResponse, error) {
	scope := c.metricsScope.Tagged(map[string]string{
		metrics.OperationTagName: metricsconstants.ShardDistributorSpectatorGetShardOwnerOperationTagName,
	})

	scope.Counter(metricsconstants.ShardDistributorSpectatorClientRequests).Inc(1)

	sw := scope.Timer(metricsconstants.ShardDistributorSpectatorClientLatency).Start()
	response, err := c.client.GetShardOwner(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		scope.Counter(metricsconstants.ShardDistributorSpectatorClientFailures).Inc(1)
	}
	return response, err
}

func (c *meteredShardDistributorClient) WatchNamespaceState(ctx context.Context, request *types.WatchNamespaceStateRequest, opts ...yarpc.CallOption) (sharddistributor.WatchNamespaceStateClient, error) {
	scope := c.metricsScope.Tagged(map[string]string{
		metrics.OperationTagName: metricsconstants.ShardDistributorSpectatorWatchNamespaceStateOperationTagName,
	})

	scope.Counter(metricsconstants.ShardDistributorSpectatorClientRequests).Inc(1)

	sw := scope.Timer(metricsconstants.ShardDistributorSpectatorClientLatency).Start()
	stream, err := c.client.WatchNamespaceState(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		scope.Counter(metricsconstants.ShardDistributorSpectatorClientFailures).Inc(1)
	}
	return stream, err
}
