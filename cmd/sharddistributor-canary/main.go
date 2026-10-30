package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/uber-go/tally"
	"github.com/urfave/cli/v3"
	"go.uber.org/fx"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/api/peer"
	"go.uber.org/yarpc/transport/grpc"
	"go.uber.org/zap"

	sharddistributorv1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary"
	canaryConfig "github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/clientcommon"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/spectatorclient"
	"github.com/cadence-workflow/shard-manager/tools/common/commoncli"
)

const (
	// Default configuration
	defaultShardDistributorEndpoint = "127.0.0.1:7943"
	defaultFixedNamespace           = "shard-distributor-canary"
	defaultEphemeralNamespace       = "shard-distributor-canary-ephemeral"
	defaultCanaryGRPCPort           = 7953 // Port for canary to receive ping requests
	defaultNumExecutors             = 1

	defaultLoadNamespace       = "shard-distributor-canary-load"
	defaultLoadInterval        = time.Hour
	defaultLoadNumShards       = 10000
	defaultLoadShardsPerSecond = 200
	defaultLoadNumExecutors    = 50
	defaultLoadHold            = 5 * time.Minute

	shardDistributorServiceName = "shard-distributor"
)

func runApp(c *cli.Command) {
	endpoint := c.String("endpoint")
	fixedNamespace := c.String("fixed-namespace")
	ephemeralNamespace := c.String("ephemeral-namespace")
	canaryGRPCPort := c.Int("canary-grpc-port")

	numFixedExecutors := c.Int("num-fixed-executors")
	numEphemeralExecutors := c.Int("num-ephemeral-executors")

	if c.IsSet("num-executors") {
		numExecutors := c.Int("num-executors")
		numFixedExecutors = numExecutors
		numEphemeralExecutors = numExecutors

	}

	loadConfig := canaryConfig.LoadConfig{
		Namespace:       c.String("load-namespace"),
		Interval:        c.Duration("load-interval"),
		NumShards:       c.Int("load-num-shards"),
		ShardsPerSecond: c.Int("load-shards-per-second"),
		NumExecutors:    c.Int("load-num-executors"),
		Hold:            c.Duration("load-hold"),
	}

	fx.New(opts(fixedNamespace, ephemeralNamespace, endpoint, canaryGRPCPort, numFixedExecutors, numEphemeralExecutors, loadConfig)).Run()
}

func opts(fixedNamespace, ephemeralNamespace, endpoint string, canaryGRPCPort int, numFixedExecutors, numEphemeral int, loadConfig canaryConfig.LoadConfig) fx.Option {
	configuration := clientcommon.Config{
		Namespaces: []clientcommon.NamespaceConfig{
			{Namespace: fixedNamespace, HeartBeatInterval: 1 * time.Second},
			{Namespace: ephemeralNamespace, HeartBeatInterval: 1 * time.Second},
		},
	}

	canaryGRPCAddress := fmt.Sprintf("127.0.0.1:%d", canaryGRPCPort)

	// Create listener for GRPC inbound
	listener, err := net.Listen("tcp", canaryGRPCAddress)
	if err != nil {
		panic(err)
	}

	transport := grpc.NewTransport()

	executorMetadata := executorclient.ExecutorMetadata{
		clientcommon.GrpcAddressMetadataKey: canaryGRPCAddress,
	}

	return fx.Options(
		fx.Supply(
			fx.Annotate(tally.NoopScope, fx.As(new(tally.Scope))),
			fx.Annotate(clock.NewRealTimeSource(), fx.As(new(clock.TimeSource))),
			configuration,
			transport,
			executorMetadata,
		),

		fx.Provide(func(peerChooser spectatorclient.SpectatorPeerChooserInterface) yarpc.Config {
			return yarpc.Config{
				Name: "shard-distributor-canary",
				Inbounds: yarpc.Inbounds{
					transport.NewInbound(listener), // Listen for incoming ping requests
				},
				Outbounds: yarpc.Outbounds{
					shardDistributorServiceName: {
						Unary:  transport.NewSingleOutbound(endpoint),
						Stream: transport.NewSingleOutbound(endpoint),
					},
					// canary-to-canary outbound uses peer chooser to route to other canary instances
					"shard-distributor-canary": {
						Unary:  transport.NewOutbound(peerChooser),
						Stream: transport.NewOutbound(peerChooser),
					},
				},
			}
		}),

		fx.Provide(
			func(t *grpc.Transport) peer.Transport { return t },
		),
		fx.Provide(
			yarpc.NewDispatcher,
			func(d *yarpc.Dispatcher) yarpc.ClientConfig { return d }, // Reprovide the dispatcher as a client config
		),
		fx.Provide(zap.NewDevelopment),
		fx.Provide(log.NewLogger),

		// We do decorate instead of Invoke because we want to start and stop the dispatcher at the
		// correct time.
		// It will start before all dependencies are started and stop after all dependencies are stopped.
		// The Decorate gives fx enough information, so it can start and stop the dispatcher at the correct time.
		//
		// It is critical to start and stop the dispatcher at the correct time.
		// Since the executors need to
		// be able to send a final "drain" request to the shard distributor before the application is stopped.
		fx.Decorate(func(
			lc fx.Lifecycle,
			dispatcher *yarpc.Dispatcher,
			server sharddistributorv1.ShardDistributorExecutorCanaryAPIYARPCServer,
		) *yarpc.Dispatcher {
			// Register canary procedures and ensure dispatcher lifecycle is managed by fx.
			dispatcher.Register(sharddistributorv1.BuildShardDistributorExecutorCanaryAPIYARPCProcedures(server))
			lc.Append(fx.StartStopHook(dispatcher.Start, dispatcher.Stop))
			return dispatcher
		}),

		// Include the canary module - it will set up spectator peer choosers and canary client
		canary.Module(canary.NamespacesNames{
			FixedNamespace:              fixedNamespace,
			EphemeralNamespace:          ephemeralNamespace,
			SharddistributorServiceName: shardDistributorServiceName,
			Config: canaryConfig.Config{
				Canary: canaryConfig.CanaryConfig{
					NumFixedExecutors:     numFixedExecutors,
					NumEphemeralExecutors: numEphemeral,
					Load:                  loadConfig,
				},
			},
		}),
	)
}

func buildCLI() *cli.Command {
	app := &cli.Command{}
	app.Name = "sharddistributor-canary"
	app.Usage = "Cadence shard distributor canary"
	app.Version = "0.0.1"

	app.Commands = []*cli.Command{
		{
			Name:  "start",
			Usage: "start shard distributor canary",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "endpoint",
					Aliases: []string{"e"},
					Value:   defaultShardDistributorEndpoint,
					Usage:   "shard distributor endpoint address",
				},
				&cli.StringFlag{
					Name:  "fixed-namespace",
					Value: defaultFixedNamespace,
					Usage: "namespace for fixed shard processing",
				},
				&cli.StringFlag{
					Name:  "ephemeral-namespace",
					Value: defaultEphemeralNamespace,
					Usage: "namespace for ephemeral shard creation testing",
				},
				&cli.IntFlag{
					Name:  "canary-grpc-port",
					Value: defaultCanaryGRPCPort,
					Usage: "port for canary to receive ping requests",
				},
				&cli.IntFlag{
					Name:  "num-executors",
					Value: defaultNumExecutors,
					Usage: "number of executors for fixed and ephemeral to start. Overrides num-fixed-executors and num-ephemeral-executors flags",
				},
				&cli.IntFlag{
					Name:  "num-fixed-executors",
					Value: defaultNumExecutors,
					Usage: "number of executors of fixed namespace to start. Don't use with num-executors",
				},
				&cli.IntFlag{
					Name:  "num-ephemeral-executors",
					Value: defaultNumExecutors,
					Usage: "number of executors of ephemeral namespace to start. Don't use with num-executors",
				},
				&cli.StringFlag{
					Name:  "load-namespace",
					Value: defaultLoadNamespace,
					Usage: "namespace for the periodic load run. A run only happens if the shard distributor has this namespace configured. Empty disables the load run",
				},
				&cli.DurationFlag{
					Name:  "load-interval",
					Value: defaultLoadInterval,
					Usage: "time between load runs, before jitter",
				},
				&cli.IntFlag{
					Name:  "load-num-shards",
					Value: defaultLoadNumShards,
					Usage: "number of shards each load run creates",
				},
				&cli.IntFlag{
					Name:  "load-shards-per-second",
					Value: defaultLoadShardsPerSecond,
					Usage: "rate at which a load run creates shards",
				},
				&cli.IntFlag{
					Name:  "load-num-executors",
					Value: defaultLoadNumExecutors,
					Usage: "number of extra executors each load run starts",
				},
				&cli.DurationFlag{
					Name:  "load-hold",
					Value: defaultLoadHold,
					Usage: "how long a load run holds its peak before releasing the shards",
				},
			},
			Action: func(_ context.Context, c *cli.Command) error {
				runApp(c)
				return nil
			},
		},
	}

	return app
}

func main() {
	app := buildCLI()
	commoncli.ExitHandler(app.Run(context.Background(), os.Args))
}
