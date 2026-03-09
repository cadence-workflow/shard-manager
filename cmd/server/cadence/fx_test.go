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
