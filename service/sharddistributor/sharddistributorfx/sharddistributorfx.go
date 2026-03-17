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

package sharddistributorfx

import (
	"go.uber.org/fx"

	sharddistributorv1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/handler"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/leader/election"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/leader/namespace"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/leader/process"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	meteredStore "github.com/cadence-workflow/shard-manager/service/sharddistributor/store/wrappers/metered"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/wrappers/grpc"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/wrappers/metered"
)

// Module provides shard distributor YARPC server implementations.
// The caller is responsible for registering procedures on the dispatcher.
var Module = fx.Module("sharddistributor",
	namespace.Module,
	election.Module,
	process.Module,
	fx.Provide(config.NewConfig),
	fx.Decorate(func(s store.Store, metricsClient metrics.Client, logger log.Logger, timeSource clock.TimeSource) store.Store {
		return meteredStore.NewStore(s, metricsClient, logger, timeSource)
	}),
	fx.Provide(provideServers),
)

type serversParams struct {
	fx.In

	ShardDistributionCfg config.ShardDistribution

	Logger        log.Logger
	MetricsClient metrics.Client
	Config        *config.Config

	TimeSource clock.TimeSource
	Store      store.Store

	Lifecycle fx.Lifecycle
}

type ServersResult struct {
	fx.Out

	APIServer      sharddistributorv1.ShardDistributorAPIYARPCServer
	ExecutorServer sharddistributorv1.ShardDistributorExecutorAPIYARPCServer
}

func provideServers(params serversParams) ServersResult {
	rawHandler := handler.NewHandler(params.Logger, params.ShardDistributionCfg, params.Store)
	wrappedHandler := metered.NewMetricsHandler(rawHandler, params.Logger, params.MetricsClient)

	executorHandler := handler.NewExecutorHandler(params.Logger, params.Store, params.TimeSource, params.ShardDistributionCfg, params.Config, params.MetricsClient)
	wrappedExecutor := metered.NewExecutorMetricsExecutor(executorHandler, params.Logger, params.MetricsClient)

	params.Lifecycle.Append(fx.StartStopHook(rawHandler.Start, rawHandler.Stop))

	return ServersResult{
		APIServer:      grpc.NewGRPCHandler(wrappedHandler),
		ExecutorServer: grpc.NewExecutorGRPCExecutor(wrappedExecutor),
	}
}
