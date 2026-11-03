package canary

import (
	"go.uber.org/fx"
	"go.uber.org/yarpc"

	sharddistributorv1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/executors"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/factory"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/handler"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/pinger"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/processor"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/processorephemeral"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/sharddistributorclient"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/sharddistributorexecutorclient"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/spectatorclient"
)

type Params struct {
	SharddistributorServiceName string

	Config config.Config
}

func Module(params Params) fx.Option {
	return fx.Module("shard-distributor-canary", opts(params))
}

func opts(params Params) fx.Option {
	cfg := params.Config.Canary

	return fx.Options(
		fx.Provide(sharddistributorv1.NewFxShardDistributorExecutorAPIYARPCClient(params.SharddistributorServiceName)),
		fx.Provide(sharddistributorv1.NewFxShardDistributorAPIYARPCClient(params.SharddistributorServiceName)),

		fx.Provide(sharddistributorclient.NewShardDistributorClient),
		fx.Provide(sharddistributorexecutorclient.NewShardDistributorExecutorClient),

		// Modules for the shard distributor canary
		fx.Provide(
			func(params factory.Params) executorclient.ShardProcessorFactory[*processor.ShardProcessor] {
				return factory.NewShardProcessorFactory(params, processor.NewShardProcessor)
			},
			func(params factory.Params) executorclient.ShardProcessorFactory[*processorephemeral.ShardProcessor] {
				return factory.NewShardProcessorFactory(params, processorephemeral.NewShardProcessor)
			},
		),

		executors.Module(cfg.Fixed, cfg.Ephemeral),

		processorephemeral.ShardCreatorModule([]string{cfg.Ephemeral.Name}),

		spectatorclient.Module(),
		fx.Provide(spectatorclient.NewSpectatorPeerChooser),

		// Create canary client using the dispatcher's client config
		fx.Provide(func(dispatcher *yarpc.Dispatcher) sharddistributorv1.ShardDistributorExecutorCanaryAPIYARPCClient {
			config := dispatcher.ClientConfig("shard-distributor-canary")
			return sharddistributorv1.NewShardDistributorExecutorCanaryAPIYARPCClient(config)
		}),

		fx.Provide(func(params pinger.Params) *pinger.Pinger {
			return pinger.NewPinger(params, cfg.Fixed.Name, 32)
		}),
		fx.Invoke(func(p *pinger.Pinger, lc fx.Lifecycle) {
			lc.Append(fx.StartStopHook(p.Start, p.Stop))
		}),

		// Register canary ping handler to receive ping requests from other executors
		fx.Provide(handler.NewPingHandler),
		fx.Provide(fx.Annotate(
			func(h *handler.PingHandler) sharddistributorv1.ShardDistributorExecutorCanaryAPIYARPCServer {
				return h
			},
		)),
		fx.Provide(sharddistributorv1.NewFxShardDistributorExecutorCanaryAPIYARPCProcedures()),

		// There is a circular dependency between the spectator client and the peer chooser, since
		// the yarpc dispatcher needs the peer chooser and the peer chooser needs the spectators, which needs the yarpc dispatcher.
		// To break the circular dependency, we set the spectators on the peer chooser here.
		fx.Invoke(func(chooser spectatorclient.SpectatorPeerChooserInterface, spectators *spectatorclient.Spectators) {
			chooser.SetSpectators(spectators)
		}),
	)
}
