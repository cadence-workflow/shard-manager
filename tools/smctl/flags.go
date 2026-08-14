package smctl

import (
	"fmt"
	"time"

	cliv3 "github.com/urfave/cli/v3"
)

// Flag names used by smctl. Connection flags are persistent on the root
// command so every subcommand inherits them, mirroring how the Cadence CLI
// exposes --address / --transport / --tls-cert-path on its root app.
const (
	FlagAddress        = "address"
	FlagTransport      = "transport"
	FlagTLSCertPath    = "tls-cert-path"
	FlagContextTimeout = "context-timeout"

	FlagNamespace  = "namespace"
	FlagShardKey   = "shard-key"
	FlagExecutorID = "executor-id"
)

// Connection defaults for talking to a locally-running shard-manager.
const (
	// smctlClientName is the YARPC caller name used by smctl. Mirrors how the
	smctlClientName = "shard-manager-cli"

	// shardManagerServiceName is the YARPC service identifier the smctl
	// dispatcher targets. The *value* must remain "shard-distributor" because
	// that is the name the server registers under (see common/service/name.go
	// `ShardDistributor = "shard-distributor"` and the production yaml
	// `service.aliases: [shard-distributor]`). The smctl-side identifier is
	// named after the umbrella shard-manager project for readability.
	shardManagerServiceName = "shard-distributor"

	// grpcDefaultAddress is the default host:port for shard-manager's gRPC
	// inbound (see config/development.yaml: rpc.grpcPort=7943).
	grpcDefaultAddress = "127.0.0.1:7943"

	// grpcTransport is the only transport currently supported. The flag is
	// kept for parity with the Cadence CLI so future tchannel/etc. variants
	// have a place to plug in.
	grpcTransport = "grpc"

	defaultContextTimeout = 10 * time.Second
)

func requiredStringFlag(cmd *cliv3.Command, name string) (string, error) {
	value := cmd.String(name)
	if value == "" {
		return "", fmt.Errorf("--%s is required", name)
	}
	return value, nil
}
