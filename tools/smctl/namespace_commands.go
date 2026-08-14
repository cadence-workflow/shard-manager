package smctl

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	cliv3 "github.com/urfave/cli/v3"

	"github.com/cadence-workflow/shard-manager/common/types"
)

// namespaceCommand returns the "namespace" command group. All of its
// subcommands operate on the namespace identified by the persistent
// --namespace/-n flag declared on the root smctl command, e.g.:
//
//	smctl -n <namespace> namespace state
//	smctl namespace state -n <namespace>
func namespaceCommand(cf ClientFactory) *cliv3.Command {
	return &cliv3.Command{
		Name:        "namespace",
		Aliases:     []string{"ns"},
		Usage:       "Inspect and manage shard-manager namespaces",
		Description: "Use --namespace/-n on the root command to identify the target namespace.",
		Commands: []*cliv3.Command{
			namespaceStateCommand(cf),
			namespaceListCommand(cf),
			namespaceForceResetCommand(cf),
		},
	}
}

// namespaceListCommand lists all namespaces in the shard-manager
func namespaceListCommand(cf ClientFactory) *cliv3.Command {
	return &cliv3.Command{
		Name:        "list",
		Aliases:     []string{"ls"},
		Usage:       "List all namespaces configured on the shard-manager fleet",
		Description: "Calls ListNamespaces on shard-manager and prints the response as a table (or JSON with --json).",
		Flags: []cliv3.Flag{
			&cliv3.BoolFlag{
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "Print the response as indented JSON instead of a table.",
			},
		},
		Action: func(ctx context.Context, cmd *cliv3.Command) error {
			return runListNamespaces(ctx, cmd, resolveWriter(cmd), cf)
		},
	}
}

func runListNamespaces(
	ctx context.Context,
	cmd *cliv3.Command,
	out io.Writer,
	cf ClientFactory,
) error {
	client, err := cf.ShardManagerClient(cmd)
	if err != nil {
		return err
	}

	callCtx, cancel := context.WithTimeout(ctx, cmd.Duration(FlagContextTimeout))
	defer cancel()

	resp, err := client.ListNamespaces(callCtx, &types.ListNamespacesRequest{})
	if err != nil {
		return fmt.Errorf("ListNamespaces: %w", err)
	}

	if cmd.Bool("json") {
		return writeIndentedJSON(out, resp)
	}

	return renderNamespacesTable(out, resp.GetNamespaces())
}

// renderNamespacesTable writes a tab-aligned table to output
func renderNamespacesTable(out io.Writer, namespaces []*types.NamespaceConfig) error {
	sorted := make([]*types.NamespaceConfig, 0, len(namespaces))
	for _, ns := range namespaces {
		if ns == nil {
			continue
		}
		sorted = append(sorted, ns)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].GetName() < sorted[j].GetName() })

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "NAME\tTYPE\tMODE\tSHARDS"); err != nil {
		return err
	}
	for _, ns := range sorted {
		shards := "-"
		if ns.GetShardNum() > 0 {
			shards = strconv.FormatInt(ns.GetShardNum(), 10)
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", ns.GetName(), ns.GetType(), ns.GetMode(), shards); err != nil {
			return err
		}
	}
	return w.Flush()
}

// namespaceStateCommand prints the current state of a namespace by calling
// shard-manager's GetNamespaceState API.
func namespaceStateCommand(cf ClientFactory) *cliv3.Command {
	return &cliv3.Command{
		Name:        "state",
		Aliases:     []string{"st"},
		Usage:       "Print the current state of a namespace",
		Description: "Calls GetNamespaceState on shard-manager and prints the response as indented JSON.",
		Action: func(ctx context.Context, cmd *cliv3.Command) error {
			return runGetNamespaceState(ctx, cmd, resolveWriter(cmd), cf)
		},
	}
}

func runGetNamespaceState(
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

	resp, err := client.GetNamespaceState(callCtx, &types.GetNamespaceStateRequest{
		Namespace: namespace,
	})
	if err != nil {
		return fmt.Errorf("GetNamespaceState: %w", err)
	}

	return writeIndentedJSON(out, resp)
}

// resolveWriter returns the writer to use for command output. urfave/cli/v3
// auto-assigns os.Stdout to every subcommand's Writer during setup, so we
// always read from the root command (which is where callers/tests set it).
func resolveWriter(cmd *cliv3.Command) io.Writer {
	if cmd != nil {
		if root := cmd.Root(); root != nil && root.Writer != nil {
			return root.Writer
		}
	}
	return os.Stdout
}

// namespaceForceResetCommand wipes etcd state for a namespace by calling
// shard-manager's ForceResetNamespace API.
func namespaceForceResetCommand(cf ClientFactory) *cliv3.Command {
	return &cliv3.Command{
		Name:    "force-reset",
		Aliases: []string{"fr"},
		Usage:   "Delete all etcd state for a namespace",
		Description: "Calls ForceResetNamespace on shard-manager. Wipes the leader key, " +
			"executor heartbeats, shard assignments, and statistics for the namespace. " +
			"Executors will re-register on their next heartbeat. ",
		Action: func(ctx context.Context, cmd *cliv3.Command) error {
			return runForceResetNamespace(ctx, cmd, resolveWriter(cmd), resolveReader(cmd), cf)
		},
	}
}

// resolveReader returns the reader to use it for interactive prompts.
func resolveReader(cmd *cliv3.Command) io.Reader {
	if cmd != nil {
		if root := cmd.Root(); root != nil && root.Reader != nil {
			return root.Reader
		}
	}
	return os.Stdin
}

func runForceResetNamespace(
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

	if err := confirmNamespace(out, in, namespace); err != nil {
		return err
	}

	client, err := cf.ShardManagerClient(cmd)
	if err != nil {
		return err
	}

	callCtx, cancel := context.WithTimeout(ctx, cmd.Duration(FlagContextTimeout))
	defer cancel()

	resp, err := client.ForceResetNamespace(callCtx, &types.ForceResetNamespaceRequest{
		Namespace: namespace,
	})
	if err != nil {
		return fmt.Errorf("ForceResetNamespace: %w", err)
	}

	return writeIndentedJSON(out, resp)
}

// confirmNamespace prompts the operator to retype the namespace name and
// returns an error if the typed value does not match exactly.
// The compare is case-sensitive
func confirmNamespace(out io.Writer, in io.Reader, namespace string) error {
	fmt.Fprintf(out,
		"This will DELETE all etcd state for namespace %q. "+
			"Retype the namespace to confirm: ", namespace,
	)

	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("read confirmation: %w", err)
		}
		return fmt.Errorf("confirmation aborted: no input received")
	}
	if got := strings.TrimSpace(scanner.Text()); got != namespace {
		return fmt.Errorf("confirmation mismatch: typed %q, expected %q", got, namespace)
	}
	return nil
}
