package smctl

import (
	"context"
	"fmt"

	cliv3 "github.com/urfave/cli/v3"

	"github.com/cadence-workflow/shard-manager/common/metrics"
)

// BuildCommand returns the root smctl command wired with ClientFactory.
func BuildCommand() *cliv3.Command {
	return BuildCommandWithFactory(NewClientFactory())
}

// BuildCommandWithFactory returns the root smctl command using the provided
// ClientFactory.
func BuildCommandWithFactory(cf ClientFactory) *cliv3.Command {
	return &cliv3.Command{
		Name:        "smctl",
		Usage:       "Command-line client for shard-manager",
		Description: "Inspect and manage shard-manager namespaces.",
		Version:     fmt.Sprintf("%s (commit %s)", metrics.ReleaseVersion, metrics.Revision),
		Flags:       rootFlags(),
		Commands: []*cliv3.Command{
			namespaceCommand(cf),
			shardCommand(cf),
			executorCommand(cf),
		},
		// After is invoked even when the action returns an error, so it is the
		// right place to release the dispatcher built on first command use.
		After: func(_ context.Context, _ *cliv3.Command) error {
			return cf.Close()
		},
	}
}

// rootFlags returns the flags exposed on the smctl root command.
// These are persistent (Local==false) so subcommands can read them regardless of
// where the flag was supplied on the command line.
func rootFlags() []cliv3.Flag {
	return []cliv3.Flag{
		&cliv3.StringFlag{
			Name:    FlagNamespace,
			Aliases: []string{"n"},
			Usage:   "namespace to operate on (required by commands that target a namespace)",
			Sources: cliv3.EnvVars("SMCTL_NAMESPACE"),
		},
		&cliv3.StringFlag{
			Name:    FlagAddress,
			Aliases: []string{"ad"},
			Usage:   "host:port for shard-manager service (default: " + grpcDefaultAddress + ")",
			Sources: cliv3.EnvVars("SMCTL_ADDRESS"),
		},
		&cliv3.StringFlag{
			Name:    FlagTransport,
			Aliases: []string{"t"},
			Usage:   "transport protocol; only 'grpc' is currently supported",
			Value:   grpcTransport,
			Sources: cliv3.EnvVars("SMCTL_TRANSPORT"),
		},
		&cliv3.StringFlag{
			Name:    FlagTLSCertPath,
			Aliases: []string{"tcp"},
			Usage:   "path to TLS server CA certificate; enables TLS when set",
			Sources: cliv3.EnvVars("SMCTL_TLS_CERT_PATH"),
		},
		&cliv3.DurationFlag{
			Name:    FlagContextTimeout,
			Aliases: []string{"ct"},
			Usage:   "timeout for each RPC call",
			Value:   defaultContextTimeout,
			Sources: cliv3.EnvVars("SMCTL_CONTEXT_TIMEOUT"),
		},
	}
}
