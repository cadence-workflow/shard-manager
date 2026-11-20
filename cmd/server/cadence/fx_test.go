package cadence

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"github.com/cadence-workflow/shard-manager/common/config"
	"github.com/cadence-workflow/shard-manager/common/service"
	"github.com/cadence-workflow/shard-manager/testflags"

	_ "github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd" // needed for shard distributor storage
)

func TestFxDependenciesForShardDistributor(t *testing.T) {
	err := fx.ValidateApp(_commonModule,
		fx.Supply(appContext{
			CfgContext: config.Context{
				Environment: "",
				Zone:        "",
			},
			ConfigDir: "",
			RootDir:   "",
		}),
		Module(service.ShortName(service.ShardDistributor)))
	require.NoError(t, err)
}

func TestShardDistributorStartStop(t *testing.T) {
	flag.Parse()
	testflags.RequireEtcd(t)

	wd, err := os.Getwd()
	require.NoError(t, err)
	app := fxtest.New(t, _commonModule,
		fx.Supply(appContext{
			CfgContext: config.Context{
				Environment: "development",
				Zone:        "",
			},
			ConfigDir: fmt.Sprintf("%s/testdata/config", wd),
			RootDir:   "",
		}),
		Module(service.ShortName(service.ShardDistributor)))
	app.RequireStart().RequireStop()
}
