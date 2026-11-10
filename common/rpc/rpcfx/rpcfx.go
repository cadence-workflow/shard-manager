package rpcfx

import (
	"fmt"
	"net"
	"strconv"

	"go.uber.org/fx"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/transport/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/cadence-workflow/shard-manager/common/config"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/common/rpc"
)

// Module provides a *yarpc.Dispatcher for the fx application.
var Module = fx.Module("rpcfx",
	fx.Provide(
		buildDispatcher,
		// Components needing the dispatcher only for client access or shutdown
		// ordering depend on the interface, so they also work under yarpcfx.
		func(d *yarpc.Dispatcher) yarpc.ClientConfig { return d },
	),
)

type params struct {
	fx.In

	ServiceFullName string `name:"service-full-name"`
	Cfg             config.Config
	Logger          log.Logger
	Lifecycle       fx.Lifecycle
}

func buildDispatcher(p params) (dispatcher *yarpc.Dispatcher, retErr error) {
	rpcCfg := p.Cfg.RPC

	listenIP, err := rpc.GetListenIP(rpcCfg)
	if err != nil {
		return nil, fmt.Errorf("get listen IP: %w", err)
	}

	grpcAddress := net.JoinHostPort(listenIP.String(), strconv.Itoa(int(rpcCfg.GRPCPort)))

	var transportOptions []grpc.TransportOption
	if rpcCfg.GRPCMaxMsgSize > 0 {
		transportOptions = append(transportOptions,
			grpc.ServerMaxRecvMsgSize(rpcCfg.GRPCMaxMsgSize),
			grpc.ClientMaxRecvMsgSize(rpcCfg.GRPCMaxMsgSize),
		)
	}
	grpcTransport := grpc.NewTransport(transportOptions...)

	listener, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		return nil, fmt.Errorf("listen on gRPC port %s: %w", grpcAddress, err)
	}
	defer func() {
		if retErr != nil {
			_ = listener.Close()
		}
	}()

	var inboundOptions []grpc.InboundOption
	inboundTLS, err := rpcCfg.TLS.ToTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("inbound TLS config: %w", err)
	}
	if inboundTLS != nil {
		inboundOptions = append(inboundOptions, grpc.InboundCredentials(credentials.NewTLS(inboundTLS)))
	}

	inbound := grpcTransport.NewInbound(listener, inboundOptions...)
	p.Logger.Info("Listening for gRPC requests", tag.Address(grpcAddress))

	dispatcher = yarpc.NewDispatcher(yarpc.Config{
		Name:     p.ServiceFullName,
		Inbounds: yarpc.Inbounds{inbound},
	})

	p.Lifecycle.Append(fx.StartStopHook(dispatcher.Start, dispatcher.Stop))

	return dispatcher, nil
}
