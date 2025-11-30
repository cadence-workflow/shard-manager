package main

import (
	"os"

	"github.com/cadence-workflow/shard-manager/cmd/server/cadence"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/tools/common/commoncli"

	_ "github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd" // needed for shard distributor shard/heartbeat and leader election
)

// main entry point for the cadence server
func main() {
	app := cadence.BuildCLI(metrics.ReleaseVersion, metrics.Revision)
	commoncli.ExitHandler(app.Run(os.Args))
}
