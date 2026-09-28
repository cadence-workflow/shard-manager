package smctl

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"sort"
	"strings"

	cliv3 "github.com/urfave/cli/v3"

	"github.com/cadence-workflow/shard-manager/common/types"
)

// executorDrainCommand drains the hosts running executors so that every
// executor reporting one of the hostnames becomes ineligible for assignment.
func executorDrainCommand(cf ClientFactory) *cliv3.Command {
	return &cliv3.Command{
		Name:  "drain",
		Usage: "Drain executor hosts so their executors are no longer assigned shards",
		Description: "Prints how many shards the executors on each host currently hold, asks for confirmation, " +
			"then calls DrainHosts on shard-manager. Repeat --hostname for multiple hosts. " +
			"The call is idempotent: an already drained host keeps its original drain metadata.",
		Flags: []cliv3.Flag{
			&cliv3.StringSliceFlag{
				Name:      FlagHostname,
				Usage:     "hostname reported by the executor heartbeat (repeat for multiple hosts)",
				Required:  true,
				Validator: nonEmptyStrings,
				Config:    cliv3.StringConfig{TrimSpace: true},
			},
			&cliv3.StringFlag{
				Name:  FlagReason,
				Usage: "reason for draining, stored with the drain record",
			},
			&cliv3.StringFlag{
				Name:  FlagDrainedBy,
				Usage: "operator recorded as having drained the hosts",
				Value: currentUsername(),
			},
			&cliv3.BoolFlag{
				Name:    FlagYes,
				Aliases: []string{"y"},
				Usage:   "skip the confirmation prompt",
			},
		},
		Action: func(ctx context.Context, cmd *cliv3.Command) error {
			return runDrainHosts(ctx, cmd, resolveWriter(cmd), resolveReader(cmd), cf)
		},
	}
}

func runDrainHosts(
	ctx context.Context,
	cmd *cliv3.Command,
	out io.Writer,
	in io.Reader,
	cf ClientFactory,
) error {
	namespace, err := requiredStringFlag(cmd, FlagNamespace)
	if err != nil {
		return err
	}
	hostnames := cmd.StringSlice(FlagHostname)

	client, err := cf.ShardManagerClient(cmd)
	if err != nil {
		return err
	}

	if !cmd.Bool(FlagYes) {
		stateCtx, cancel := context.WithTimeout(ctx, cmd.Duration(FlagContextTimeout))
		state, err := client.GetNamespaceState(stateCtx, &types.GetNamespaceStateRequest{
			Namespace: namespace,
		})
		cancel()
		if err != nil {
			return fmt.Errorf("GetNamespaceState: %w", err)
		}
		if err := confirmDrainHosts(out, in, hostnames, state.GetExecutors()); err != nil {
			return err
		}
	}

	hosts := make([]*types.DrainedHost, 0, len(hostnames))
	for _, hostname := range hostnames {
		hosts = append(hosts, &types.DrainedHost{
			Hostname:  hostname,
			DrainedBy: cmd.String(FlagDrainedBy),
			Reason:    cmd.String(FlagReason),
		})
	}
	req := &types.DrainHostsRequest{
		Namespace: namespace,
		Hosts:     hosts,
	}

	callCtx, cancel := context.WithTimeout(ctx, cmd.Duration(FlagContextTimeout))
	defer cancel()

	if err := client.DrainHosts(callCtx, req); err != nil {
		return fmt.Errorf("DrainHosts: %w", err)
	}

	return writeIndentedJSON(out, req)
}

// confirmDrainHosts prints the shards currently held by executors on each
// host and returns an error unless the operator answers yes.
func confirmDrainHosts(out io.Writer, in io.Reader, hostnames []string, executors []*types.NamespaceExecutorState) error {
	executorsByHost := make(map[string][]*types.NamespaceExecutorState)
	for _, ex := range executors {
		hostname := ex.GetHostMetadata().GetHostName()
		if hostname == "" {
			continue
		}
		executorsByHost[hostname] = append(executorsByHost[hostname], ex)
	}

	for _, hostname := range hostnames {
		hostExecutors := executorsByHost[hostname]
		if len(hostExecutors) == 0 {
			fmt.Fprintf(out, "Host %q has no executors registered in this namespace.\n", hostname)
			continue
		}
		sort.Slice(hostExecutors, func(i, j int) bool {
			return hostExecutors[i].GetExecutorID() < hostExecutors[j].GetExecutorID()
		})
		total := 0
		for _, ex := range hostExecutors {
			total += len(ex.GetAssignedShards())
		}
		fmt.Fprintf(out, "Host %q currently holds %d shard(s):\n", hostname, total)
		for _, ex := range hostExecutors {
			fmt.Fprintf(out, "  executor %s: %d shard(s)\n", ex.GetExecutorID(), len(ex.GetAssignedShards()))
		}
	}
	fmt.Fprint(out, "Proceed with draining? [y/N]: ")

	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("read confirmation: %w", err)
		}
		return fmt.Errorf("drain aborted: no input received")
	}
	switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
	case "y", "yes":
		return nil
	default:
		return fmt.Errorf("drain aborted")
	}
}

func currentUsername() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return os.Getenv("USER")
}

// executorUndrainCommand removes hosts from the drained list so their
// executors can be assigned shards again.
func executorUndrainCommand(cf ClientFactory) *cliv3.Command {
	return &cliv3.Command{
		Name:  "undrain",
		Usage: "Undrain executor hosts so their executors can be assigned shards again",
		Description: "Calls UndrainHosts on shard-manager and prints the hostnames this call actually undrained. " +
			"Repeat --hostname for multiple hosts. The call is idempotent.",
		Flags: []cliv3.Flag{
			&cliv3.StringSliceFlag{
				Name:      FlagHostname,
				Usage:     "hostname to undrain (repeat for multiple hosts)",
				Required:  true,
				Validator: nonEmptyStrings,
				Config:    cliv3.StringConfig{TrimSpace: true},
			},
		},
		Action: func(ctx context.Context, cmd *cliv3.Command) error {
			return runUndrainHosts(ctx, cmd, resolveWriter(cmd), cf)
		},
	}
}

func runUndrainHosts(
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

	resp, err := client.UndrainHosts(callCtx, &types.UndrainHostsRequest{
		Namespace: namespace,
		Hostnames: cmd.StringSlice(FlagHostname),
	})
	if err != nil {
		return fmt.Errorf("UndrainHosts: %w", err)
	}

	return writeIndentedJSON(out, resp)
}

// executorListDrainedCommand lists hosts currently drained in a namespace.
func executorListDrainedCommand(cf ClientFactory) *cliv3.Command {
	return &cliv3.Command{
		Name:        "list-drained",
		Usage:       "List executor hosts currently drained in a namespace",
		Description: "Calls GetDrainedHosts on shard-manager and prints the response, including drain metadata, as JSON.",
		Action: func(ctx context.Context, cmd *cliv3.Command) error {
			return runGetDrainedHosts(ctx, cmd, resolveWriter(cmd), cf)
		},
	}
}

func runGetDrainedHosts(
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

	resp, err := client.GetDrainedHosts(callCtx, &types.GetDrainedHostsRequest{
		Namespace: namespace,
	})
	if err != nil {
		return fmt.Errorf("GetDrainedHosts: %w", err)
	}

	return writeIndentedJSON(out, resp)
}
