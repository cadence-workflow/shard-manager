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
		Usage:       "Inspect and manage shard-manager shards",
		Description: "Use --namespace/-n on the root command to identify the target namespace.",
		Commands: []*cliv3.Command{
			shardInspectCommand(cf),
			shardDrainCommand(cf),
			shardUndrainCommand(cf),
			shardListDrainedCommand(cf),
		},
	}
}

// shardInspectCommand prints shard owner metadata by calling InspectShard API
func shardInspectCommand(cf ClientFactory) *cliv3.Command {
	return &cliv3.Command{
		Name:        "inspect",
		Usage:       "Inspect the current owner of a shard from storage",
		Description: "Calls InspectShard on shard-manager and prints the response as JSON.",
		Flags: []cliv3.Flag{
			&cliv3.StringFlag{
				Name:      FlagShardKey,
				Usage:     "shard key to look up",
				Required:  true,
				Validator: nonEmptyString,
				Config:    cliv3.StringConfig{TrimSpace: true},
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

	client, err := cf.ShardManagerClient(cmd)
	if err != nil {
		return err
	}

	callCtx, cancel := context.WithTimeout(ctx, cmd.Duration(FlagContextTimeout))
	defer cancel()

	resp, err := client.InspectShard(callCtx, &types.GetShardOwnerRequest{
		Namespace: namespace,
		ShardKey:  cmd.String(FlagShardKey),
	})
	if err != nil {
		return fmt.Errorf("InspectShard: %w", err)
	}

	return writeIndentedJSON(out, resp)
}

// shardDrainCommand marks shards as drained so they are unassigned on the
// next rebalance and stay ineligible for assignment until undrained.
func shardDrainCommand(cf ClientFactory) *cliv3.Command {
	return &cliv3.Command{
		Name:        "drain",
		Usage:       "Mark shards as drained so they are unassigned and not reassigned",
		Description: "Calls DrainShards on shard-manager. Repeat --shard-key for multiple shards. The call is idempotent.",
		Flags: []cliv3.Flag{
			&cliv3.StringSliceFlag{
				Name:      FlagShardKey,
				Usage:     "shard key to drain (repeat for multiple keys)",
				Required:  true,
				Validator: nonEmptyStrings,
				Config:    cliv3.StringConfig{TrimSpace: true},
			},
		},
		Action: func(ctx context.Context, cmd *cliv3.Command) error {
			return runDrainShards(ctx, cmd, resolveWriter(cmd), cf)
		},
	}
}

func runDrainShards(
	ctx context.Context,
	cmd *cliv3.Command,
	out io.Writer,
	cf ClientFactory,
) error {
	namespace, shardKeys, err := requireNamespaceAndShardKeys(cmd)
	if err != nil {
		return err
	}

	client, err := cf.ShardManagerClient(cmd)
	if err != nil {
		return err
	}

	callCtx, cancel := context.WithTimeout(ctx, cmd.Duration(FlagContextTimeout))
	defer cancel()

	req := &types.DrainShardsRequest{
		Namespace: namespace,
		ShardKeys: shardKeys,
	}
	if err := client.DrainShards(callCtx, req); err != nil {
		return fmt.Errorf("DrainShards: %w", err)
	}

	return writeIndentedJSON(out, req)
}

// shardUndrainCommand removes shards from the drained set so they can be assigned again.
func shardUndrainCommand(cf ClientFactory) *cliv3.Command {
	return &cliv3.Command{
		Name:        "undrain",
		Usage:       "Remove shards from the drained set so they can be assigned again",
		Description: "Calls UndrainShards on shard-manager and prints the shard keys this call actually undrained. Repeat --shard-key for multiple shards. The call is idempotent.",
		Flags: []cliv3.Flag{
			&cliv3.StringSliceFlag{
				Name:      FlagShardKey,
				Usage:     "shard key to undrain (repeat for multiple keys)",
				Required:  true,
				Validator: nonEmptyStrings,
				Config:    cliv3.StringConfig{TrimSpace: true},
			},
		},
		Action: func(ctx context.Context, cmd *cliv3.Command) error {
			return runUndrainShards(ctx, cmd, resolveWriter(cmd), cf)
		},
	}
}

func runUndrainShards(
	ctx context.Context,
	cmd *cliv3.Command,
	out io.Writer,
	cf ClientFactory,
) error {
	namespace, shardKeys, err := requireNamespaceAndShardKeys(cmd)
	if err != nil {
		return err
	}

	client, err := cf.ShardManagerClient(cmd)
	if err != nil {
		return err
	}

	callCtx, cancel := context.WithTimeout(ctx, cmd.Duration(FlagContextTimeout))
	defer cancel()

	resp, err := client.UndrainShards(callCtx, &types.UndrainShardsRequest{
		Namespace: namespace,
		ShardKeys: shardKeys,
	})
	if err != nil {
		return fmt.Errorf("UndrainShards: %w", err)
	}

	return writeIndentedJSON(out, resp)
}

// shardListDrainedCommand lists shards currently marked as drained in a namespace.
func shardListDrainedCommand(cf ClientFactory) *cliv3.Command {
	return &cliv3.Command{
		Name:        "list-drained",
		Usage:       "List shards currently marked as drained in a namespace",
		Description: "Calls GetDrainedShards on shard-manager and prints the response as JSON.",
		Action: func(ctx context.Context, cmd *cliv3.Command) error {
			return runGetDrainedShards(ctx, cmd, resolveWriter(cmd), cf)
		},
	}
}

func runGetDrainedShards(
	ctx context.Context,
	cmd *cliv3.Command,
	out io.Writer,
	cf ClientFactory,
) error {
	namespace, err := requiredStringFlag(cmd, FlagNamespace)
	if err != nil {
		return err
	}

	client, err := cf.ShardManagerClient(cmd)
	if err != nil {
		return err
	}

	callCtx, cancel := context.WithTimeout(ctx, cmd.Duration(FlagContextTimeout))
	defer cancel()

	resp, err := client.GetDrainedShards(callCtx, &types.GetDrainedShardsRequest{
		Namespace: namespace,
	})
	if err != nil {
		return fmt.Errorf("GetDrainedShards: %w", err)
	}

	return writeIndentedJSON(out, resp)
}

func requireNamespaceAndShardKeys(cmd *cliv3.Command) (string, []string, error) {
	namespace, err := requiredStringFlag(cmd, FlagNamespace)
	if err != nil {
		return "", nil, err
	}
	return namespace, cmd.StringSlice(FlagShardKey), nil
}
