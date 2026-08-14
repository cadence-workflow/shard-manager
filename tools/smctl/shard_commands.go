package smctl

import (
	"context"
	"fmt"
	"io"

	cliv3 "github.com/urfave/cli/v3"

	"github.com/cadence-workflow/shard-manager/common/types"
)

func shardCommand(cf ClientFactory) *cliv3.Command {
	return &cliv3.Command{
		Name:        "shard",
		Aliases:     []string{"sh"},
		Usage:       "Inspect and manage shard-manager shards",
		Description: "Use --namespace/-n on the root command to identify the target namespace.",
		Commands: []*cliv3.Command{
			shardInspectCommand(cf),
		},
	}
}

// shardInspectCommand prints shard owner metadata by calling InspectShard API
func shardInspectCommand(cf ClientFactory) *cliv3.Command {
	return &cliv3.Command{
		Name:        "inspect",
		Aliases:     []string{"in"},
		Usage:       "Inspect the current owner of a shard from storage",
		Description: "Calls InspectShard on shard-manager and prints the response as JSON.",
		Flags: []cliv3.Flag{
			&cliv3.StringFlag{
				Name:    FlagShardKey,
				Aliases: []string{"sk"},
				Usage:   "shard key to look up",
			},
		},
		Action: func(ctx context.Context, cmd *cliv3.Command) error {
			return runInspectShard(ctx, cmd, resolveWriter(cmd), cf)
		},
	}
}

func runInspectShard(
	ctx context.Context,
	cmd *cliv3.Command,
	out io.Writer,
	cf ClientFactory,
) error {
	namespace, err := requiredStringFlag(cmd, FlagNamespace)
	if err != nil {
		return err
	}

	shardKey, err := requiredStringFlag(cmd, FlagShardKey)
	if err != nil {
		return err
	}

	client, err := cf.ShardManagerClient(cmd)
	if err != nil {
		return err
	}

	callCtx, cancel := context.WithTimeout(ctx, cmd.Duration(FlagContextTimeout))
	defer cancel()

	resp, err := client.InspectShard(callCtx, &types.GetShardOwnerRequest{
		Namespace: namespace,
		ShardKey:  shardKey,
	})
	if err != nil {
		return fmt.Errorf("InspectShard: %w", err)
	}

	return writeIndentedJSON(out, resp)
}
