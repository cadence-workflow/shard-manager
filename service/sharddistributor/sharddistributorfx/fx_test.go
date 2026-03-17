package sharddistributorfx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"go.uber.org/yarpc"

	sharddistributorv1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/dynamicconfig"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

func TestFxServiceStartStop(t *testing.T) {
	defer goleak.VerifyNone(t)

	testDispatcher := yarpc.NewDispatcher(yarpc.Config{Name: "test"})
	ctrl := gomock.NewController(t)
	app := fxtest.New(t,
		testlogger.Module(t),
		fx.Provide(
			func() metrics.Client { return metrics.NewNoopMetricsClient() },
			func() *yarpc.Dispatcher { return testDispatcher },
			func() *dynamicconfig.Collection {
				return dynamicconfig.NewNopCollection()
			},
			fx.Annotated{Target: func() string { return "testHost" }, Name: "hostname"},
			func() store.Elector {
				return store.NewMockElector(ctrl)
			},
			func() store.Store {
				return store.NewMockStore(ctrl)
			},
			func() config.ShardDistribution {
				return config.ShardDistribution{}
			},
			func() clock.TimeSource {
				return clock.NewMockedTimeSource()
			},
		),
		Module,
		fx.Invoke(func(
			dispatcher *yarpc.Dispatcher,
			apiServer sharddistributorv1.ShardDistributorAPIYARPCServer,
			executorServer sharddistributorv1.ShardDistributorExecutorAPIYARPCServer,
		) {
			dispatcher.Register(sharddistributorv1.BuildShardDistributorAPIYARPCProcedures(apiServer))
			dispatcher.Register(sharddistributorv1.BuildShardDistributorExecutorAPIYARPCProcedures(executorServer))
		}),
	)
	app.RequireStart().RequireStop()
	assert.True(t, len(testDispatcher.Introspect().Procedures) > 3)
}
