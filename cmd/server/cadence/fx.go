// The MIT License (MIT)

// Copyright (c) 2017-2020 Uber Technologies Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package cadence

import (
	"context"

	"github.com/uber-go/tally"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/cadence-workflow/shard-manager/common"
	"github.com/cadence-workflow/shard-manager/common/clock/clockfx"
	"github.com/cadence-workflow/shard-manager/common/config"
	"github.com/cadence-workflow/shard-manager/common/dynamicconfig"
	"github.com/cadence-workflow/shard-manager/common/dynamicconfig/dynamicconfigfx"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/logfx"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/common/metrics"
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

// Module provides a cadence server initialization with root components.
// AppParams allows to provide optional/overrides for implementation specific dependencies.
func Module(serviceName string) fx.Option {
	if serviceName == service.ShortName(service.ShardDistributor) {
		return fx.Options(
			fx.Supply(serviceContext{
				Name:     serviceName,
				FullName: service.FullName(serviceName),
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
			sharddistributorfx.Module)
	}
	return fx.Options(
		fx.Supply(serviceContext{
			Name:     serviceName,
			FullName: service.FullName(serviceName),
		}),
		fx.Provide(NewApp),
		// empty invoke so fx won't drop the application from the dependencies.
		fx.Invoke(func(a *App) {}),
	)
}

type AppParams struct {
	fx.In

	Service       string `name:"service"`
	AppContext    config.Context
	Config        config.Config
	Logger        log.Logger
	LifeCycle     fx.Lifecycle
	DynamicConfig dynamicconfig.Client
	Scope         tally.Scope
	MetricsClient metrics.Client
}

// NewApp created a new Application from pre initalized config and logger.
func NewApp(params AppParams) *App {
	app := &App{
		cfg:           params.Config,
		logger:        params.Logger,
		service:       params.Service,
		dynamicConfig: params.DynamicConfig,
		scope:         params.Scope,
		metricsClient: params.MetricsClient,
	}

	params.LifeCycle.Append(fx.StartHook(app.verifySchema))
	params.LifeCycle.Append(fx.StartStopHook(app.Start, app.Stop))
	return app
}

// App is a fx application that registers itself into fx.Lifecycle and runs.
// It is done implicitly, since it provides methods Start and Stop which are picked up by fx.
type App struct {
	cfg           config.Config
	rootDir       string
	logger        log.Logger
	dynamicConfig dynamicconfig.Client
	scope         tally.Scope
	metricsClient metrics.Client

	daemon  common.Daemon
	service string
}

func (a *App) Start(_ context.Context) error {
	a.daemon = newServer(a.service, a.cfg, a.logger, a.dynamicConfig, a.scope, a.metricsClient)
	a.daemon.Start()
	return nil
}

func (a *App) Stop(ctx context.Context) error {
	a.daemon.Stop()
	return nil
}

func (a *App) verifySchema(ctx context.Context) error {
	// shard-manager only uses ETCD, no schema verification needed for SQL/Cassandra
	return nil
}

type serviceContext struct {
	fx.Out

	Name     string `name:"service"`
	FullName string `name:"service-full-name"`
}
