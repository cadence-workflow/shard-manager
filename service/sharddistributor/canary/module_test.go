package canary

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	ubergomock "go.uber.org/mock/gomock"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/api/peer"
	"go.uber.org/yarpc/api/transport/transporttest"
	"go.uber.org/yarpc/transport/grpc"
	"go.uber.org/yarpc/yarpctest"
	"go.uber.org/zap/zaptest"

	sharddistributorv1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient"
)

func TestModule(t *testing.T) {
	// Create mocks
	ctrl := gomock.NewController(t)
	uberCtrl := ubergomock.NewController(t)
	mockLogger := log.NewNoop()

	mockClientConfig := transporttest.NewMockClientConfig(ctrl)
	transport := grpc.NewTransport()
	outbound := transport.NewOutbound(yarpctest.NewFakePeerList())

	mockClientConfig.EXPECT().Caller().Return("test-executor").AnyTimes()
	mockClientConfig.EXPECT().Service().Return("shard-distributor").AnyTimes()
	mockClientConfig.EXPECT().GetUnaryOutbound().Return(outbound).AnyTimes()

	mockClientConfigProvider := transporttest.NewMockClientConfigProvider(ctrl)
	mockClientConfigProvider.EXPECT().ClientConfig("shard-distributor").Return(mockClientConfig).AnyTimes()

	// Create executor yarpc client mock
	mockYARPCClient := executorclient.NewMockShardDistributorExecutorAPIYARPCClient(uberCtrl)
	mockYARPCClient.EXPECT().
		Heartbeat(ubergomock.Any(), ubergomock.Any(), ubergomock.Any()).
		Return(&sharddistributorv1.HeartbeatResponse{}, nil).
		AnyTimes()

	cfg := config.Config{
		Canary: config.CanaryConfig{
			Fixed:     config.NamespaceConfig{Name: "shard-distributor-canary"},
			Ephemeral: config.NamespaceConfig{Name: "shard-distributor-canary-ephemeral"},
		},
	}

	// Create a mock dispatcher with the required outbound
	dispatcher := yarpc.NewDispatcher(yarpc.Config{
		Name: "test-canary",
		Outbounds: yarpc.Outbounds{
			"shard-distributor-canary": {
				Unary: outbound,
			},
		},
	})

	// Create a test app with the library, check that it starts and stops
	fxtest.New(t,
		fx.Supply(
			fx.Annotate(tally.NoopScope, fx.As(new(tally.Scope))),
			fx.Annotate(clock.NewMockedTimeSource(), fx.As(new(clock.TimeSource))),
			fx.Annotate(mockLogger, fx.As(new(log.Logger))),
			fx.Annotate(mockClientConfigProvider, fx.As(new(yarpc.ClientConfig))),
			fx.Annotate(transport, fx.As(new(peer.Transport))),
			zaptest.NewLogger(t),
			cfg.Canary.ClientConfig(),
			dispatcher,
		),
		// Replacing the real YARPC client with mock to handle the draining heartbeat
		fx.Decorate(func() sharddistributorv1.ShardDistributorExecutorAPIYARPCClient {
			return mockYARPCClient
		}),
		Module(Params{
			SharddistributorServiceName: "shard-distributor",
			Config:                      cfg,
		}),
	).RequireStart().RequireStop()
}
