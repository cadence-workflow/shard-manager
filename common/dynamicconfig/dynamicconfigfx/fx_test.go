package dynamicconfigfx

import (
	"testing"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"github.com/cadence-workflow/shard-manager/common/config"
	"github.com/cadence-workflow/shard-manager/common/dynamicconfig"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
)

func TestModule(t *testing.T) {
	app := fxtest.New(t,
		testlogger.Module(t),
		fx.Provide(
			func() config.Config {
				return config.Config{
					ClusterGroupMetadata: &config.ClusterGroupMetadata{},
				}
			},
			func() fxRoot {
				return fxRoot{
					RootDir: "../../../",
				}
			},
			metrics.NewNoopMetricsClient,
		),
		Module,
		fx.Invoke(func(c dynamicconfig.Client) {}),
	)
	app.RequireStart().RequireStop()
}

type fxRoot struct {
	fx.Out

	RootDir string `name:"root-dir"`
}
