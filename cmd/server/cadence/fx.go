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
	"go.uber.org/fx"
	"go.uber.org/zap"

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

type serviceContext struct {
	fx.Out

	FullName string `name:"service-full-name"`
}
