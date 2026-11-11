package sharddistributor

import (
	"context"

	"go.uber.org/yarpc"

	"github.com/cadence-workflow/shard-manager/common/types"
)

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination interface_mock.go -self_package github.com/cadence-workflow/shard-manager/client/sharddistributor
//go:generate gowrap gen -g -p . -i Client -t ../templates/retry.tmpl -o ../wrappers/retryable/sharddistributor_generated.go -v client=ShardDistributor
//go:generate gowrap gen -g -p . -i Client -t ../templates/metered.tmpl -o ../wrappers/metered/sharddistributor_generated.go -v client=ShardDistributor
//go:generate gowrap gen -g -p . -i Client -t ../templates/errorinjectors.tmpl -o ../wrappers/errorinjectors/sharddistributor_generated.go -v client=ShardDistributor
//go:generate gowrap gen -g -p . -i Client -t ../templates/grpc.tmpl -o ../wrappers/grpc/sharddistributor_generated.go -v client=ShardDistributor -v package=sharddistributorv1 -v path=github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1 -v prefix=ShardDistributor
//go:generate gowrap gen -g -p . -i Client -t ../templates/timeout.tmpl -o ../wrappers/timeout/sharddistributor_generated.go -v client=ShardDistributor

type Client interface {
	GetShardOwner(context.Context, *types.GetShardOwnerRequest, ...yarpc.CallOption) (*types.GetShardOwnerResponse, error)
	InspectShard(context.Context, *types.GetShardOwnerRequest, ...yarpc.CallOption) (*types.GetShardOwnerResponse, error)
	GetNamespaceState(context.Context, *types.GetNamespaceStateRequest, ...yarpc.CallOption) (*types.GetNamespaceStateResponse, error)
	GetFullNamespaceState(context.Context, *types.GetFullNamespaceStateRequest, ...yarpc.CallOption) (*types.GetFullNamespaceStateResponse, error)
	GetExecutorState(context.Context, *types.GetExecutorStateRequest, ...yarpc.CallOption) (*types.GetExecutorStateResponse, error)
	WatchNamespaceState(context.Context, *types.WatchNamespaceStateRequest, ...yarpc.CallOption) (WatchNamespaceStateClient, error)
	GetNamespaceStateIfNewer(context.Context, *types.GetNamespaceStateIfNewerRequest, ...yarpc.CallOption) (*types.GetNamespaceStateIfNewerResponse, error)
	DrainShards(context.Context, *types.DrainShardsRequest, ...yarpc.CallOption) error
	UndrainShards(context.Context, *types.UndrainShardsRequest, ...yarpc.CallOption) (*types.UndrainShardsResponse, error)
	GetDrainedShards(context.Context, *types.GetDrainedShardsRequest, ...yarpc.CallOption) (*types.GetDrainedShardsResponse, error)
	DrainHosts(context.Context, *types.DrainHostsRequest, ...yarpc.CallOption) error
	UndrainHosts(context.Context, *types.UndrainHostsRequest, ...yarpc.CallOption) (*types.UndrainHostsResponse, error)
	GetDrainedHosts(context.Context, *types.GetDrainedHostsRequest, ...yarpc.CallOption) (*types.GetDrainedHostsResponse, error)
	ForceResetNamespace(context.Context, *types.ForceResetNamespaceRequest, ...yarpc.CallOption) (*types.ForceResetNamespaceResponse, error)
	ListNamespaces(context.Context, *types.ListNamespacesRequest, ...yarpc.CallOption) (*types.ListNamespacesResponse, error)
}

type WatchNamespaceStateClient interface {
	Context() context.Context
	Recv(...yarpc.StreamOption) (*types.WatchNamespaceStateResponse, error)
	CloseSend(...yarpc.StreamOption) error
}
