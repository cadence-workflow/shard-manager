package handler

import (
	"context"

	"github.com/cadence-workflow/shard-manager/common"
	"github.com/cadence-workflow/shard-manager/common/types"
)

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination interfaces_mock.go
//go:generate gowrap gen -g -p . -i Handler -t ../../templates/grpc.tmpl -o ../wrappers/grpc/grpc_handler_generated.go -v handler=GRPC -v package=sharddistributorv1 -v path=github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1 -v prefix=ShardDistributor
//go:generate gowrap gen -g -p . -i Executor -t ../../templates/grpc.tmpl -o ../wrappers/grpc/grpc_executor_generated.go -v handler=ExecutorGRPC -v package=sharddistributorv1 -v path=github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1 -v prefix=ShardDistributorExecutor
//go:generate gowrap gen -g -p . -i Handler -t ../templates/metered.tmpl -o ../wrappers/metered/api_generated.go -v handler=Metrics
//go:generate gowrap gen -g -p . -i Executor -t ../templates/metered.tmpl -o ../wrappers/metered/executor_generated.go -v handler=ExecutorMetrics

// Handler is the interface for shard distributor handler
type Handler interface {
	common.Daemon

	Health(context.Context) (*types.HealthStatus, error)

	GetShardOwner(context.Context, *types.GetShardOwnerRequest) (*types.GetShardOwnerResponse, error)

	InspectShard(context.Context, *types.GetShardOwnerRequest) (*types.GetShardOwnerResponse, error)

	GetNamespaceState(context.Context, *types.GetNamespaceStateRequest) (*types.GetNamespaceStateResponse, error)

	GetFullNamespaceState(context.Context, *types.GetFullNamespaceStateRequest) (*types.GetFullNamespaceStateResponse, error)

	GetExecutorState(context.Context, *types.GetExecutorStateRequest) (*types.GetExecutorStateResponse, error)

	WatchNamespaceState(*types.WatchNamespaceStateRequest, WatchNamespaceStateServer) error

	GetNamespaceStateIfNewer(context.Context, *types.GetNamespaceStateIfNewerRequest) (*types.GetNamespaceStateIfNewerResponse, error)

	ListNamespaces(context.Context, *types.ListNamespacesRequest) (*types.ListNamespacesResponse, error)

	DrainShards(context.Context, *types.DrainShardsRequest) error
	UndrainShards(context.Context, *types.UndrainShardsRequest) (*types.UndrainShardsResponse, error)
	GetDrainedShards(context.Context, *types.GetDrainedShardsRequest) (*types.GetDrainedShardsResponse, error)

	DrainHosts(context.Context, *types.DrainHostsRequest) error
	UndrainHosts(context.Context, *types.UndrainHostsRequest) (*types.UndrainHostsResponse, error)
	GetDrainedHosts(context.Context, *types.GetDrainedHostsRequest) (*types.GetDrainedHostsResponse, error)

	ForceResetNamespace(context.Context, *types.ForceResetNamespaceRequest) (*types.ForceResetNamespaceResponse, error)
}

type Executor interface {
	Heartbeat(context.Context, *types.ExecutorHeartbeatRequest) (*types.ExecutorHeartbeatResponse, error)
}

type WatchNamespaceStateServer interface {
	Context() context.Context
	Send(*types.WatchNamespaceStateResponse) error
}
