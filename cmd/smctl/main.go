package main

import (
	"context"
	"os"

	"github.com/cadence-workflow/shard-manager/tools/common/commoncli"
	"github.com/cadence-workflow/shard-manager/tools/smctl"
)

func main() {
	cmd := smctl.BuildCommand()
	commoncli.ExitHandler(cmd.Run(context.Background(), os.Args))
}
