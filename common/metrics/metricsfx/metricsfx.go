package metricsfx

import (
	"github.com/uber-go/tally"
	"go.uber.org/fx"

	"github.com/cadence-workflow/shard-manager/common/config"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/service"
)

// Module provides metrics client for fx application.
var Module = fx.Module("metricsfx",
	fx.Provide(buildClient))

// ModuleForExternalScope provides metrics client for fx application when tally.Scope is created outside.
var ModuleForExternalScope = fx.Module("metricsfx",
	fx.Provide(buildClientFromTally))

type clientParams struct {
	fx.In

	Logger       log.Logger
	SvcCfg       config.Service
	MigrationCfg metrics.MigrationConfig
}

type clientResult struct {
	fx.Out

	Scope  tally.Scope
	Client metrics.Client
}

func buildClient(params clientParams) clientResult {
	scope := params.SvcCfg.Metrics.NewScope(params.Logger, service.ShardDistributor)
	return clientResult{
		Scope:  scope,
		Client: metrics.NewClient(scope, metrics.ShardDistributor, params.MigrationCfg),
	}
}

func buildClientFromTally(scope tally.Scope, migrationCfg metrics.MigrationConfig) metrics.Client {
	// Ensure external scope path emits with the expected service tag.
	scope = scope.Tagged(map[string]string{
		"service": service.ShardDistributor,
	})
	return metrics.NewClient(scope, metrics.ShardDistributor, migrationCfg)
}
