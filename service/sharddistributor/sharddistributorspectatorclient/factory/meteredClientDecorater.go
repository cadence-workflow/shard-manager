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

func (c *meteredShardDistributorClient) InspectShard(ctx context.Context, request *types.GetShardOwnerRequest, opts ...yarpc.CallOption) (*types.GetShardOwnerResponse, error) {
	scope := c.metricsScope.Tagged(map[string]string{
		metrics.OperationTagName: metricsconstants.ShardDistributorSpectatorInspectShardOperationTagName,
	})

	scope.Counter(metricsconstants.ShardDistributorSpectatorClientRequests).Inc(1)

	sw := scope.Timer(metricsconstants.ShardDistributorSpectatorClientLatency).Start()
	response, err := c.client.InspectShard(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		scope.Counter(metricsconstants.ShardDistributorSpectatorClientFailures).Inc(1)
	}
	return response, err
}

func (c *meteredShardDistributorClient) GetNamespaceState(ctx context.Context, request *types.GetNamespaceStateRequest, opts ...yarpc.CallOption) (*types.GetNamespaceStateResponse, error) {
	scope := c.metricsScope.Tagged(map[string]string{
		metrics.OperationTagName: metricsconstants.ShardDistributorSpectatorGetNamespaceStateOperationTagName,
	})

	scope.Counter(metricsconstants.ShardDistributorSpectatorClientRequests).Inc(1)

	sw := scope.Timer(metricsconstants.ShardDistributorSpectatorClientLatency).Start()
	response, err := c.client.GetNamespaceState(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		scope.Counter(metricsconstants.ShardDistributorSpectatorClientFailures).Inc(1)
	}
	return response, err
}

func (c *meteredShardDistributorClient) GetExecutorState(ctx context.Context, request *types.GetExecutorStateRequest, opts ...yarpc.CallOption) (*types.GetExecutorStateResponse, error) {
	scope := c.metricsScope.Tagged(map[string]string{
		metrics.OperationTagName: metricsconstants.ShardDistributorSpectatorGetExecutorStateOperationTagName,
	})

	scope.Counter(metricsconstants.ShardDistributorSpectatorClientRequests).Inc(1)

	sw := scope.Timer(metricsconstants.ShardDistributorSpectatorClientLatency).Start()
	response, err := c.client.GetExecutorState(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		scope.Counter(metricsconstants.ShardDistributorSpectatorClientFailures).Inc(1)
	}
	return response, err
}

func (c *meteredShardDistributorClient) ListNamespaces(ctx context.Context, request *types.ListNamespacesRequest, opts ...yarpc.CallOption) (*types.ListNamespacesResponse, error) {
	scope := c.metricsScope.Tagged(map[string]string{
		metrics.OperationTagName: metricsconstants.ShardDistributorSpectatorListNamespacesOperationTagName,
	})

	scope.Counter(metricsconstants.ShardDistributorSpectatorClientRequests).Inc(1)

	sw := scope.Timer(metricsconstants.ShardDistributorSpectatorClientLatency).Start()
	response, err := c.client.ListNamespaces(ctx, request, opts...)
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

func (c *meteredShardDistributorClient) DrainShards(ctx context.Context, request *types.DrainShardsRequest, opts ...yarpc.CallOption) (*types.DrainShardsResponse, error) {
	scope := c.metricsScope.Tagged(map[string]string{
		metrics.OperationTagName: metricsconstants.ShardDistributorSpectatorDrainShardsOperationTagName,
	})

	scope.Counter(metricsconstants.ShardDistributorSpectatorClientRequests).Inc(1)

	sw := scope.Timer(metricsconstants.ShardDistributorSpectatorClientLatency).Start()
	response, err := c.client.DrainShards(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		scope.Counter(metricsconstants.ShardDistributorSpectatorClientFailures).Inc(1)
	}
	return response, err
}

func (c *meteredShardDistributorClient) UndrainShards(ctx context.Context, request *types.UndrainShardsRequest, opts ...yarpc.CallOption) (*types.UndrainShardsResponse, error) {
	scope := c.metricsScope.Tagged(map[string]string{
		metrics.OperationTagName: metricsconstants.ShardDistributorSpectatorUndrainShardsOperationTagName,
	})

	scope.Counter(metricsconstants.ShardDistributorSpectatorClientRequests).Inc(1)

	sw := scope.Timer(metricsconstants.ShardDistributorSpectatorClientLatency).Start()
	response, err := c.client.UndrainShards(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		scope.Counter(metricsconstants.ShardDistributorSpectatorClientFailures).Inc(1)
	}
	return response, err
}

func (c *meteredShardDistributorClient) GetDrainedShards(ctx context.Context, request *types.GetDrainedShardsRequest, opts ...yarpc.CallOption) (*types.GetDrainedShardsResponse, error) {
	scope := c.metricsScope.Tagged(map[string]string{
		metrics.OperationTagName: metricsconstants.ShardDistributorSpectatorGetDrainedShardsOperationTagName,
	})

	scope.Counter(metricsconstants.ShardDistributorSpectatorClientRequests).Inc(1)

	sw := scope.Timer(metricsconstants.ShardDistributorSpectatorClientLatency).Start()
	response, err := c.client.GetDrainedShards(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		scope.Counter(metricsconstants.ShardDistributorSpectatorClientFailures).Inc(1)
	}
	return response, err
}

func (c *meteredShardDistributorClient) ForceResetNamespace(ctx context.Context, request *types.ForceResetNamespaceRequest, opts ...yarpc.CallOption) (*types.ForceResetNamespaceResponse, error) {
	scope := c.metricsScope.Tagged(map[string]string{
		metrics.OperationTagName: metricsconstants.ShardDistributorSpectatorForceResetNamespaceOperationTagName,
	})

	scope.Counter(metricsconstants.ShardDistributorSpectatorClientRequests).Inc(1)

	sw := scope.Timer(metricsconstants.ShardDistributorSpectatorClientLatency).Start()
	response, err := c.client.ForceResetNamespace(ctx, request, opts...)
	sw.Stop()

	if err != nil {
		scope.Counter(metricsconstants.ShardDistributorSpectatorClientFailures).Inc(1)
	}
	return response, err
}
