// The MIT License (MIT)

// Copyright (c) 2017-2020 Uber Technologies Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

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
	fx.Provide(buildDispatcher),
)

type params struct {
	fx.In

	ServiceFullName string `name:"service-full-name"`
	Cfg             config.Config
	Logger          log.Logger
	Lifecycle       fx.Lifecycle
}

func buildDispatcher(p params) (*yarpc.Dispatcher, error) {
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

	dispatcher := yarpc.NewDispatcher(yarpc.Config{
		Name:     p.ServiceFullName,
		Inbounds: yarpc.Inbounds{inbound},
	})

	p.Lifecycle.Append(fx.StartStopHook(dispatcher.Start, dispatcher.Stop))

	return dispatcher, nil
}
