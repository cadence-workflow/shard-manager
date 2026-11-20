package cadence

import (
	"go.uber.org/fx"
	"go.uber.org/yarpc"
	"go.uber.org/zap"

	sharddistributorv1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1"
	"github.com/cadence-workflow/shard-manager/common/clock/clockfx"
	"github.com/cadence-workflow/shard-manager/common/config"
	"github.com/cadence-workflow/shard-manager/common/dynamicconfig/dynamicconfigfx"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/logfx"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/common/metrics/metricsfx"
	"github.com/cadence-workflow/shard-manager/common/rpc/rpcfx"
	"github.com/cadence-workflow/shard-manager/common/service"
	shardDistributorCfg "github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/sharddistributorfx"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd"
)

var _commonModule = fx.Options(
	config.Module,
	dynamicconfigfx.Module,
	logfx.Module,
	metricsfx.Module,
	clockfx.Module)

// Module provides a shard-manager server initialization with root components.
func Module(serviceName string) fx.Option {
	if serviceName != service.ShortName(service.ShardDistributor) {
		panic("shard-manager only supports sharddistributor service")
	}
	return fx.Options(
		fx.Supply(serviceContext{
			FullName: service.ShardDistributor,
		}),
		fx.Provide(func(cfg config.Config) shardDistributorCfg.ShardDistribution {
			return cfg.ShardDistribution
		}),
		// Decorate both logger so all components use proper service name.
		fx.Decorate(func(z *zap.Logger, l log.Logger) (*zap.Logger, log.Logger) {
			return z.With(zap.String("service", service.ShardDistributor)), l.WithTags(tag.Service(service.ShardDistributor))
		}),

		etcd.Module,

		rpcfx.Module,
		sharddistributorfx.Module,
		fx.Invoke(registerProcedures))
}

func registerProcedures(
	dispatcher *yarpc.Dispatcher,
	apiServer sharddistributorv1.ShardDistributorAPIYARPCServer,
	executorServer sharddistributorv1.ShardDistributorExecutorAPIYARPCServer,
) {
	dispatcher.Register(sharddistributorv1.BuildShardDistributorAPIYARPCProcedures(apiServer))
	dispatcher.Register(sharddistributorv1.BuildShardDistributorExecutorAPIYARPCProcedures(executorServer))
}

type serviceContext struct {
	fx.Out

	FullName string `name:"service-full-name"`
}
