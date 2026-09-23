package executorstore

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdclient"
)

// The client is consumed by name, so it has to be provided as one.
type namedClient struct {
	fx.Out

	Client etcdclient.Client `name:"executorstore"`
}

// Tests that this part of the fx graph builds.
func TestModule_ProvidesTheStore(t *testing.T) {
	ctrl := gomock.NewController(t)

	require.NoError(t, fx.ValidateApp(
		Module,
		fx.Provide(func() namedClient { return namedClient{Client: etcdclient.NewMockClient(ctrl)} }),
		fx.Provide(func() etcdclient.ExecutorStoreConfig { return etcdclient.ExecutorStoreConfig{} }),
		fx.Provide(func() log.Logger { return testlogger.New(t) }),
		fx.Provide(func() clock.TimeSource { return clock.NewRealTimeSource() }),
		fx.Provide(func() metrics.Client { return metrics.NewNoopMetricsClient() }),
		fx.Provide(func() *config.Config { return &config.Config{} }),
		fx.Invoke(func(store.Store) {}),
	))
}
