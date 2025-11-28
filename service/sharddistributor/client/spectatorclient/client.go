package spectatorclient

import (
	"context"
	"fmt"

	"github.com/uber-go/tally"
	"go.uber.org/fx"

	sharddistributorv1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1"
	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/client/wrappers/grpc"
	"github.com/cadence-workflow/shard-manager/client/wrappers/retryable"
	timeoutwrapper "github.com/cadence-workflow/shard-manager/client/wrappers/timeout"
	"github.com/cadence-workflow/shard-manager/common"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/clientcommon"
)

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination interface_mock.go . Spectator

type Spectator interface {
	Start(ctx context.Context) error
	Stop()

	// GetShardOwner returns the owner of a shard. It first checks the local cache,
	// and if not found, falls back to querying the shard distributor directly.
	GetShardOwner(ctx context.Context, shardKey string) (string, error)
}

type Params struct {
	fx.In

	YarpcClient  sharddistributorv1.ShardDistributorAPIYARPCClient
	MetricsScope tally.Scope
	Logger       log.Logger
	Config       clientcommon.Config
	TimeSource   clock.TimeSource
}

// NewSpectatorWithNamespace creates a spectator for a specific namespace
func NewSpectatorWithNamespace(params Params, namespace string) (Spectator, error) {
	return newSpectatorImpl(params, namespace)
}

// NewSpectator creates a spectator for the single namespace in config
func NewSpectator(params Params) (Spectator, error) {
	cfg, err := params.Config.GetSingleConfig()
	if err != nil {
		return nil, err
	}
	return newSpectatorImpl(params, cfg.Namespace)
}

func newSpectatorImpl(params Params, namespace string) (Spectator, error) {
	// Get config for the specified namespace
	namespaceConfig, err := params.Config.GetConfigForNamespace(namespace)
	if err != nil {
		return nil, fmt.Errorf("get config for namespace %s: %w", namespace, err)
	}

	return newSpectatorWithConfig(params, namespaceConfig)
}

func newSpectatorWithConfig(params Params, namespaceConfig *clientcommon.NamespaceConfig) (Spectator, error) {
	// Create the wrapped shard distributor client
	shardDistributorClient, err := createShardDistributorClient(params.YarpcClient, params.MetricsScope)
	if err != nil {
		return nil, fmt.Errorf("create shard distributor client: %w", err)
	}

	impl := &spectatorImpl{
		namespace:  namespaceConfig.Namespace,
		config:     *namespaceConfig,
		client:     shardDistributorClient,
		logger:     params.Logger,
		scope:      params.MetricsScope,
		timeSource: params.TimeSource,
	}
	// Set WaitGroup to 1 to block until first state is received
	impl.firstStateWG.Add(1)

	return impl, nil
}

func createShardDistributorClient(yarpcClient sharddistributorv1.ShardDistributorAPIYARPCClient, metricsScope tally.Scope) (sharddistributor.Client, error) {
	// Wrap the YARPC client with GRPC wrapper
	client := grpc.NewShardDistributorClient(yarpcClient)

	// Add timeout wrapper
	client = timeoutwrapper.NewShardDistributorClient(client, timeoutwrapper.ShardDistributorDefaultTimeout)

	// Add metered wrapper
	if metricsScope != nil {
		client = NewMeteredShardDistributorClient(client, metricsScope)
	}

	// Add retry wrapper
	client = retryable.NewShardDistributorClient(
		client,
		common.CreateShardDistributorServiceRetryPolicy(),
		common.IsServiceTransientError,
	)

	return client, nil
}

// Module creates a spectator module using auto-selection (single namespace only)
func Module() fx.Option {
	return fx.Module("shard-distributor-spectator-client",
		fx.Provide(NewSpectator),
		fx.Invoke(func(spectator Spectator, lc fx.Lifecycle) {
			lc.Append(fx.StartStopHook(spectator.Start, spectator.Stop))
		}),
	)
}

// ModuleWithNamespace creates a spectator module for a specific namespace
func ModuleWithNamespace(namespace string) fx.Option {
	return fx.Module(fmt.Sprintf("shard-distributor-spectator-client-%s", namespace),
		fx.Provide(func(params Params) (Spectator, error) {
			return NewSpectatorWithNamespace(params, namespace)
		}),
		fx.Invoke(func(spectator Spectator, lc fx.Lifecycle) {
			lc.Append(fx.StartStopHook(spectator.Start, spectator.Stop))
		}),
	)
}
