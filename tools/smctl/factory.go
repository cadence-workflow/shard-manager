//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination factory_mock.go -self_package github.com/cadence-workflow/shard-manager/tools/smctl

package smctl

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	cliv3 "github.com/urfave/cli/v3"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/api/transport"
	"go.uber.org/yarpc/peer"
	"go.uber.org/yarpc/peer/hostport"
	"go.uber.org/yarpc/transport/grpc"
	"google.golang.org/grpc/credentials"

	sharddistributorv1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1"
	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	grpcClient "github.com/cadence-workflow/shard-manager/client/wrappers/grpc"
	"github.com/cadence-workflow/shard-manager/common"
	"github.com/cadence-workflow/shard-manager/common/types"
)

// ClientFactory builds the RPC clients used by smctl subcommands. The
// dispatcher is created lazily on first use.
type ClientFactory interface {
	// ShardManagerClient returns a client to the shard-manager service that
	// talks to the address resolved from the persistent connection flags on
	// cmd. The returned client is the upstream sharddistributor.Client because
	// shard-manager exposes the same RPC surface as the historical
	// shard-distributor service.
	ShardManagerClient(cmd *cliv3.Command) (sharddistributor.Client, error)
	// Close releases any underlying transports. Safe to call when no client
	// has been built.
	Close() error
}

type clientFactory struct {
	dispatcher *yarpc.Dispatcher
}

// NewClientFactory creates a ClientFactory backed by a lazily-created YARPC
// gRPC dispatcher.
func NewClientFactory() ClientFactory {
	return &clientFactory{}
}

func (b *clientFactory) ShardManagerClient(cmd *cliv3.Command) (sharddistributor.Client, error) {
	if err := b.ensureDispatcher(cmd); err != nil {
		return nil, fmt.Errorf("create shard-manager client dependency: %w", err)
	}
	clientConfig := b.dispatcher.ClientConfig(shardManagerServiceName)
	return grpcClient.NewShardDistributorClient(
		sharddistributorv1.NewShardDistributorAPIYARPCClient(clientConfig),
	), nil
}

func (b *clientFactory) Close() error {
	if b.dispatcher == nil {
		return nil
	}
	err := b.dispatcher.Stop()
	b.dispatcher = nil
	return err
}

func (b *clientFactory) ensureDispatcher(cmd *cliv3.Command) error {
	if b.dispatcher != nil {
		return nil
	}
	d, err := newClientDispatcher(cmd)
	if err != nil {
		return fmt.Errorf("dispatcher (--%s %q): %w", FlagAddress, cmd.String(FlagAddress), err)
	}
	b.dispatcher = d
	return nil
}

func newClientDispatcher(cmd *cliv3.Command) (*yarpc.Dispatcher, error) {
	switch tp := cmd.String(FlagTransport); tp {
	case "", grpcTransport:
	default:
		return nil, fmt.Errorf("unsupported --%s %q (only %q is supported)", FlagTransport, tp, grpcTransport)
	}

	hostPort := cmd.String(FlagAddress)
	if hostPort == "" {
		hostPort = grpcDefaultAddress
	}

	grpcTransportInst := grpc.NewTransport()
	var outbounds transport.Outbounds

	if tlsCertPath := cmd.String(FlagTLSCertPath); tlsCertPath != "" {
		caCert, err := os.ReadFile(tlsCertPath)
		if err != nil {
			return nil, fmt.Errorf("unable to read server CA certificate (--%s %q): %w", FlagTLSCertPath, tlsCertPath, err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse server CA certificate (--%s %q)", FlagTLSCertPath, tlsCertPath)
		}
		tlsCreds := credentials.NewTLS(&tls.Config{RootCAs: caCertPool})
		tlsChooser := peer.NewSingle(
			hostport.Identify(hostPort),
			grpcTransportInst.NewDialer(grpc.DialerCredentials(tlsCreds)),
		)
		outbounds = transport.Outbounds{Unary: grpcTransportInst.NewOutbound(tlsChooser)}
	} else {
		outbounds = transport.Outbounds{Unary: grpcTransportInst.NewSingleOutbound(hostPort)}
	}

	dispatcher := yarpc.NewDispatcher(yarpc.Config{
		Name:      smctlClientName,
		Outbounds: yarpc.Outbounds{shardManagerServiceName: outbounds},
		OutboundMiddleware: yarpc.OutboundMiddleware{
			Unary: &versionMiddleware{},
		},
	})

	if err := dispatcher.Start(); err != nil {
		if stopErr := dispatcher.Stop(); stopErr != nil {
			return nil, fmt.Errorf("start dispatcher: %w (stop also failed: %v)", err, stopErr)
		}
		return nil, fmt.Errorf("start dispatcher: %w", err)
	}
	return dispatcher, nil
}

// versionMiddleware injects the standard caller-identification headers on
// every outgoing request so shard-manager's authorization, rate-limiting and
// observability layers recognize the caller as a CLI rather than rejecting
// it as anonymous.
type versionMiddleware struct{}

func (vm *versionMiddleware) Call(ctx context.Context, request *transport.Request, out transport.UnaryOutbound) (*transport.Response, error) {
	request.Headers = request.Headers.
		With(common.CallerTypeHeaderName, types.CallerTypeCLI.String())
	return out.Call(ctx, request)
}
