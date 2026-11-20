package sharddistributorfx

import (
	"go.uber.org/fx"
	"go.uber.org/yarpc"

	sharddistributorv1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1"
	"github.com/cadence-workflow/shard-manager/common/authorization"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/handler"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/leader/election"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/leader/namespace"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/leader/process"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/namespaces"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/namespaces/manager"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	meteredStore "github.com/cadence-workflow/shard-manager/service/sharddistributor/store/wrappers/metered"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/wrappers/accesscontrolled"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/wrappers/grpc"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/wrappers/metered"
)

// Module provides shard distributor YARPC server implementations.
// The caller is responsible for registering procedures on the dispatcher.
var Module = fx.Module("sharddistributor",
	namespaces.Module,
	namespace.Module,
	election.Module,
	process.Module,
	manager.Module,
	fx.Provide(config.NewConfig),
	fx.Decorate(func(s store.Store, metricsClient metrics.Client, logger log.Logger, timeSource clock.TimeSource) store.Store {
		return meteredStore.NewStore(s, metricsClient, logger, timeSource)
	}),
	fx.Provide(provideServers),
)

type serversParams struct {
	fx.In

	Namespaces       *namespaces.Registry
	NamespaceManager *manager.Manager

	Logger        log.Logger
	MetricsClient metrics.Client
	Config        *config.Config

	TimeSource clock.TimeSource
	Store      store.Store
	// DispatcherOrdering enforces lifecycle ordering so handler.Stop runs before
	// dispatcher.Stop. Typed as ClientConfig so yarpcfx-style apps, which expose
	// their dispatcher only as one, can satisfy it.
	DispatcherOrdering yarpc.ClientConfig

	Authorizer authorization.Authorizer `optional:"true"`

	Lifecycle fx.Lifecycle
}

type ServersResult struct {
	fx.Out

	APIServer      sharddistributorv1.ShardDistributorAPIYARPCServer
	ExecutorServer sharddistributorv1.ShardDistributorExecutorAPIYARPCServer
}

func provideServers(params serversParams) ServersResult {
	rawHandler := handler.NewHandler(params.Logger, params.TimeSource, params.NamespaceManager, params.Store)
	wrappedHandler := metered.NewMetricsHandler(rawHandler, params.Logger, params.MetricsClient)
	wrappedHandler = accesscontrolled.NewHandler(wrappedHandler, params.Authorizer)

	executorHandler := handler.NewExecutorHandler(params.Logger, params.Namespaces, params.Store, params.TimeSource, params.Config, params.MetricsClient)
	wrappedExecutor := metered.NewExecutorMetricsExecutor(executorHandler, params.Logger, params.MetricsClient)

	params.Lifecycle.Append(fx.StartStopHook(rawHandler.Start, rawHandler.Stop))

	return ServersResult{
		APIServer:      grpc.NewGRPCHandler(wrappedHandler),
		ExecutorServer: grpc.NewExecutorGRPCExecutor(wrappedExecutor),
	}
}
