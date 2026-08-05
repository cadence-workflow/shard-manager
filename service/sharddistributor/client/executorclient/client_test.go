package executorclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/mock/gomock"
	"go.uber.org/yarpc"
	"go.uber.org/zap"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/clientcommon"
)

func TestModule(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockShardProcessorFactory := NewMockShardProcessorFactory[*MockShardProcessor](ctrl)
	shardDistributorExecutorClient := NewMockClient(ctrl)
	shardDistributorExecutorClient.EXPECT().
		Heartbeat(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&types.ExecutorHeartbeatResponse{}, nil).
		AnyTimes()
	// Example config
	config := clientcommon.Config{
		Namespaces: []clientcommon.NamespaceConfig{
			{
				Namespace:         "test-namespace",
				HeartBeatInterval: 5 * time.Second,
			},
		},
	}

	// Create a test app with the library, check that it starts and stops
	fxtest.New(t,
		fx.Provide(func() Client {
			return shardDistributorExecutorClient
		}),
		fx.Supply(
			fx.Annotate(tally.NoopScope, fx.As(new(tally.Scope))),
			zap.NewNop(),
			fx.Annotate(mockShardProcessorFactory, fx.As(new(ShardProcessorFactory[*MockShardProcessor]))),
			fx.Annotate(clock.NewMockedTimeSource(), fx.As(new(clock.TimeSource))),
			config,
		),
		Module[*MockShardProcessor](),
	).RequireStart().RequireStop()
}

func TestNewExecutor_ExecutorID(t *testing.T) {
	ctrl := gomock.NewController(t)

	var heartbeatedIDs []string
	client := NewMockClient(ctrl)
	client.EXPECT().
		Heartbeat(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, request *types.ExecutorHeartbeatRequest, _ ...yarpc.CallOption) (*types.ExecutorHeartbeatResponse, error) {
			heartbeatedIDs = append(heartbeatedIDs, request.ExecutorID)
			return &types.ExecutorHeartbeatResponse{}, nil
		}).AnyTimes()

	params := Params[*MockShardProcessor]{
		ExecutorClient:        client,
		MetricsScope:          tally.NoopScope,
		Logger:                zap.NewNop(),
		ShardProcessorFactory: NewMockShardProcessorFactory[*MockShardProcessor](ctrl),
		TimeSource:            clock.NewMockedTimeSource(),
		Config: clientcommon.Config{
			Namespaces: []clientcommon.NamespaceConfig{
				{
					Namespace:         "test-namespace",
					HeartBeatInterval: 5 * time.Second,
				},
			},
		},
	}

	first, err := NewExecutor(params)
	require.NoError(t, err)
	second, err := NewExecutor(params)
	require.NoError(t, err)

	assert.NotEmpty(t, first.GetExecutorID())
	assert.Equal(t, first.GetExecutorID(), first.GetExecutorID(), "executor ID should be stable across calls")
	assert.NotEqual(t, first.GetExecutorID(), second.GetExecutorID(), "each executor should heartbeat under its own ID")

	// The reported ID must be the one the executor actually heartbeats with,
	// since the executor and its heartbeat manager hold separate copies of it.
	require.NoError(t, first.(*executorImpl[*MockShardProcessor]).heartbeater.DrainingHeartbeat())
	require.NoError(t, second.(*executorImpl[*MockShardProcessor]).heartbeater.DrainingHeartbeat())
	assert.Equal(t, []string{first.GetExecutorID(), second.GetExecutorID()}, heartbeatedIDs)
}

// Create distinct mock processor types for testing multiple namespaces
type MockShardProcessor1 struct {
	*MockShardProcessor
}

type MockShardProcessor2 struct {
	*MockShardProcessor
}

func TestModuleWithNamespace(t *testing.T) {
	ctrl := gomock.NewController(t)
	shardDistributorExecutorClient := NewMockClient(ctrl)
	shardDistributorExecutorClient.EXPECT().
		Heartbeat(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&types.ExecutorHeartbeatResponse{}, nil).
		AnyTimes()

	mockFactory1 := NewMockShardProcessorFactory[*MockShardProcessor1](ctrl)
	mockFactory2 := NewMockShardProcessorFactory[*MockShardProcessor2](ctrl)

	// Multi-namespace config
	config := clientcommon.Config{
		Namespaces: []clientcommon.NamespaceConfig{
			{
				Namespace:         "namespace1",
				HeartBeatInterval: 5 * time.Second,
			},
			{
				Namespace:         "namespace2",
				HeartBeatInterval: 10 * time.Second,
			},
		},
	}

	// Create a test app with two namespace-specific modules using different processor types
	fxtest.New(t,
		fx.Provide(func() Client {
			return shardDistributorExecutorClient
		}),
		fx.Supply(
			fx.Annotate(tally.NoopScope, fx.As(new(tally.Scope))),
			zap.NewNop(),
			fx.Annotate(clock.NewMockedTimeSource(), fx.As(new(clock.TimeSource))),
			fx.Annotate(mockFactory1, fx.As(new(ShardProcessorFactory[*MockShardProcessor1]))),
			fx.Annotate(mockFactory2, fx.As(new(ShardProcessorFactory[*MockShardProcessor2]))),
			config,
		),
		// Two namespace-specific modules with different processor types
		ModuleWithNamespace[*MockShardProcessor1]("namespace1"),
		ModuleWithNamespace[*MockShardProcessor2]("namespace2"),
	).RequireStart().RequireStop()
}
