package smctl

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	cliv3 "github.com/urfave/cli/v3"

	"github.com/cadence-workflow/shard-manager/common/types"
)

// executorCommand returns the "executor" command group. All of its
// subcommands operate on executors within the namespace identified by
// the persistent --namespace/-n flag declared on the root smctl command, e.g.:
//
//	smctl -n <namespace> executor list
//	smctl executor list -n <namespace>
func executorCommand(cf ClientFactory) *cliv3.Command {
	return &cliv3.Command{
		Name:        "executor",
		Aliases:     []string{"ex"},
		Usage:       "Inspect and manage shard-manager executors",
		Description: "Use --namespace/-n on the root command to identify the target namespace.",
		Commands: []*cliv3.Command{
			executorListCommand(cf),
			executorStateCommand(cf),
		},
	}
}

// executorListCommand lists executors registered in a namespace
func executorListCommand(cf ClientFactory) *cliv3.Command {
	return &cliv3.Command{
		Name:        "list",
		Aliases:     []string{"ls"},
		Usage:       "List executors registered in a namespace",
		Description: "Prints an executor summary as a table (or JSON with --json). Per-executor shard assignments are collapsed to a count; use `executor state` for full detail.",
		Flags: []cliv3.Flag{
			&cliv3.BoolFlag{
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "Print the executor list as indented JSON instead of a table.",
			},
		},
		Action: func(ctx context.Context, cmd *cliv3.Command) error {
			return runListExecutors(ctx, cmd, resolveWriter(cmd), cf)
		},
	}
}

func runListExecutors(
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

	summaries := projectExecutorSummaries(resp.GetExecutors())

	if cmd.Bool("json") {
		envelope := executorListEnvelope{
			Namespace: resp.GetNamespace(),
			Executors: summaries,
		}
		return writeIndentedJSON(out, envelope)
	}

	return renderExecutorsTable(out, summaries)
}

// executorStateCommand prints one executor's state by calling GetExecutorState
func executorStateCommand(cf ClientFactory) *cliv3.Command {
	return &cliv3.Command{
		Name:        "state",
		Aliases:     []string{"st"},
		Usage:       "Print the current state of an executor",
		Description: "Calls GetExecutorState on shard-manager and prints the response as indented JSON.",
		Flags: []cliv3.Flag{
			&cliv3.StringFlag{
				Name:    FlagExecutorID,
				Aliases: []string{"eid"},
				Usage:   "executor ID to look up",
			},
		},
		Action: func(ctx context.Context, cmd *cliv3.Command) error {
			return runGetExecutorState(ctx, cmd, resolveWriter(cmd), cf)
		},
	}
}

func runGetExecutorState(
	ctx context.Context,
	cmd *cliv3.Command,
	out io.Writer,
	cf ClientFactory,
) error {
	namespace, err := requiredStringFlag(cmd, FlagNamespace)
	if err != nil {
		return err
	}

	executorID, err := requiredStringFlag(cmd, FlagExecutorID)
	if err != nil {
		return err
	}

	client, err := cf.ShardManagerClient(cmd)
	if err != nil {
		return err
	}

	callCtx, cancel := context.WithTimeout(ctx, cmd.Duration(FlagContextTimeout))
	defer cancel()

	resp, err := client.GetExecutorState(callCtx, &types.GetExecutorStateRequest{
		Namespace:  namespace,
		ExecutorID: executorID,
	})
	if err != nil {
		return fmt.Errorf("GetExecutorState: %w", err)
	}

	return writeIndentedJSON(out, resp)
}

type executorSummary struct {
	ExecutorID    string            `json:"executorId"`
	Status        string            `json:"status"`
	LastHeartbeat time.Time         `json:"lastHeartbeat"`
	ShardCount    int               `json:"shardCount"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type executorListEnvelope struct {
	Namespace string            `json:"namespace"`
	Executors []executorSummary `json:"executors"`
}

func projectExecutorSummaries(executors []*types.NamespaceExecutorState) []executorSummary {
	summaries := make([]executorSummary, 0, len(executors))
	for _, ex := range executors {
		if ex == nil {
			continue
		}
		summaries = append(summaries, executorSummary{
			ExecutorID:    ex.GetExecutorID(),
			Status:        shortExecutorStatus(ex.GetStatus()),
			LastHeartbeat: ex.GetLastHeartbeat(),
			ShardCount:    len(ex.GetAssignedShards()),
			Metadata:      ex.GetMetadata(),
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].ExecutorID < summaries[j].ExecutorID
	})
	return summaries
}

func renderExecutorsTable(out io.Writer, summaries []executorSummary) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "EXECUTOR_ID\tSTATUS\tLAST_HEARTBEAT\tSHARDS\tMETADATA"); err != nil {
		return err
	}
	for _, s := range summaries {
		heartbeat := "-"
		if !s.LastHeartbeat.IsZero() {
			heartbeat = s.LastHeartbeat.UTC().Format(time.RFC3339)
		}
		if _, err := fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\n",
			s.ExecutorID,
			s.Status,
			heartbeat,
			strconv.Itoa(s.ShardCount),
			formatMetadata(s.Metadata),
		); err != nil {
			return err
		}
	}
	return w.Flush()
}

// shortExecutorStatus strips the redundant "ExecutorStatus" prefix from the
// enumer-generated String() output for the table view
func shortExecutorStatus(s types.ExecutorStatus) string {
	return strings.TrimPrefix(s.String(), "ExecutorStatus")
}

// formatMetadata renders metadata as "k1=v1,k2=v2" with keys sorted so the
// output is stable across runs
func formatMetadata(metadata map[string]string) string {
	if len(metadata) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(metadata))
	for k := range metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, metadata[k]))
	}
	return strings.Join(parts, ",")
}
