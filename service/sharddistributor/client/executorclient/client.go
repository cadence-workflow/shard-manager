package executorclient

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	"go.uber.org/yarpc"
	"go.uber.org/zap"

	timeoutwrapper "github.com/cadence-workflow/shard-manager/client/wrappers/timeout"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/clientcommon"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient/heartbeat"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient/metricsconstants"
)

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination interface_mock.go . ShardProcessorFactory,ShardProcessor,Executor,Client

// ErrShardProcessNotFound is returned by GetShardProcess when this host is not
// assigned the requested shard. Callers that interpret shard ownership should
// treat this as an ownership-loss signal rather than an internal error.
var ErrShardProcessNotFound = errors.New("shard process not found")

const maxHostnameLength = 128

type Client interface {
	Heartbeat(context.Context, *types.ExecutorHeartbeatRequest, ...yarpc.CallOption) (*types.ExecutorHeartbeatResponse, error)
}

type ExecutorMetadata map[string]string

type ShardReport struct {
	ShardLoad float64
	Status    types.ShardStatus
}

type ShardProcessor interface {
	Start(ctx context.Context) error
	Stop()
	GetShardReport() ShardReport
	SetShardStatus(types.ShardStatus)
}

type ShardProcessorFactory[SP ShardProcessor] interface {
	NewShardProcessor(shardID string) (SP, error)
}

type Executor[SP ShardProcessor] interface {
	Start(ctx context.Context)
	Stop()

	GetShardProcess(ctx context.Context, shardID string) (SP, error)

	// Get the namespace this executor is responsible for
	GetNamespace() string

	// GetExecutorID returns the ID this executor identifies itself with when it
	// heartbeats. It is generated per executor instance, so it changes whenever
	// the process restarts.
	GetExecutorID() string

	// Set metadata for the executor
	SetMetadata(metadata map[string]string)
	// Get the current metadata of the executor
	GetMetadata() map[string]string
}

type Params[SP ShardProcessor] struct {
	fx.In

	ExecutorClient        Client
	MetricsScope          tally.Scope
	Logger                *zap.Logger
	ShardProcessorFactory ShardProcessorFactory[SP]
	Config                clientcommon.Config
	TimeSource            clock.TimeSource
	Metadata              ExecutorMetadata                 `optional:"true"`
	DrainObserver         clientcommon.DrainSignalObserver `optional:"true"`

	// Enabled gates whether the executor heartbeats to the shard distributor.
	// It is consulted on every heartbeat tick so callers can roll the executor
	// in and out at runtime (e.g. a percentage-based onboarding guard). When
	// nil the executor is always enabled.
	Enabled func() bool `optional:"true"`
}

// NewExecutorWithNamespace creates an executor for a specific namespace
func NewExecutorWithNamespace[SP ShardProcessor](params Params[SP], namespace string) (Executor[SP], error) {
	// Validate the config first
	if err := params.Config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Get config for the specified namespace
	namespaceConfig, err := params.Config.GetConfigForNamespace(namespace)
	if err != nil {
		return nil, fmt.Errorf("get config for namespace %s: %w", namespace, err)
	}

	return newExecutorWithConfig(params, namespaceConfig)
}

// NewExecutor creates an executor using auto-selection (single namespace only)
func NewExecutor[SP ShardProcessor](params Params[SP]) (Executor[SP], error) {
	// Validate the config first
	if err := params.Config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Auto-select if there's only one namespace
	namespaceConfig, err := params.Config.GetSingleConfig()
	if err != nil {
		return nil, fmt.Errorf("auto-select namespace: %w", err)
	}

	return newExecutorWithConfig(params, namespaceConfig)
}

func newExecutorWithConfig[SP ShardProcessor](params Params[SP], namespaceConfig *clientcommon.NamespaceConfig) (Executor[SP], error) {
	shardDistributorClient, err := createShardDistributorExecutorClient(params.ExecutorClient, params.MetricsScope)
	if err != nil {
		return nil, fmt.Errorf("create shard distributor executor client: %w", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("get hostname: %w", err)
	}

	uniqueID := uuid.New().String()
	executorID := buildExecutorID(hostname, uniqueID)

	metricsScope := params.MetricsScope.Tagged(map[string]string{
		metrics.OperationTagName: metricsconstants.ShardDistributorExecutorOperationTagName,
		"namespace":              namespaceConfig.Namespace,
	})

	hostMetricsScope := metricsScope.Tagged(map[string]string{
		"host": hostname,
	})

	enabled := params.Enabled
	if enabled == nil {
		params.Logger.Warn("no Enabled func supplied to executor; defaulting to always enabled",
			zap.String("namespace", namespaceConfig.Namespace))
		enabled = func() bool { return true }
	}

	executor := &executorImpl[SP]{
		logger:                params.Logger,
		shardProcessorFactory: params.ShardProcessorFactory,
		heartBeatInterval:     namespaceConfig.HeartBeatInterval,
		ttlShard:              namespaceConfig.TTLShard,
		namespace:             namespaceConfig.Namespace,
		executorID:            executorID,
		timeSource:            params.TimeSource,
		stopC:                 make(chan struct{}),
		metrics:               metricsScope,
		metadata: syncExecutorMetadata{
			data: params.Metadata,
		},
		drainObserver: params.DrainObserver,
		enabled:       enabled,
	}

	executor.heartbeater = heartbeat.NewManager(
		shardDistributorClient,
		namespaceConfig.Namespace,
		executorID,
		executor,
		hostMetricsScope,
		namespaceConfig.HeartBeatInterval,
	)

	return executor, nil
}

func buildExecutorID(hostname, uniqueID string) string {
	// Executor IDs are etcd path segments, so they cannot contain slashes.
	hostname = strings.ReplaceAll(hostname, "/", "_")

	// Trim the hostname to ensure it's not unbounded
	if len(hostname) > maxHostnameLength {
		hostname = hostname[:maxHostnameLength]
	}

	executorID := hostname + "@" + uniqueID

	return executorID
}

func createShardDistributorExecutorClient(client Client, metricsScope tally.Scope) (Client, error) {
	if client == nil {
		return nil, fmt.Errorf("executor client is nil: ensure the shard-distributor YARPC outbound is configured")
	}

	shardDistributorExecutorClient := timeoutwrapper.NewShardDistributorExecutorClient(client, timeoutwrapper.ShardDistributorExecutorDefaultTimeout)

	if metricsScope != nil {
		shardDistributorExecutorClient = NewMeteredShardDistributorExecutorClient(shardDistributorExecutorClient, metricsScope)
	}

	return shardDistributorExecutorClient, nil
}

func Module[SP ShardProcessor]() fx.Option {
	return fx.Module("shard-distributor-executor-client",
		fx.Provide(NewExecutor[SP]),
		fx.Invoke(func(executor Executor[SP], lc fx.Lifecycle) {
			lc.Append(fx.StartStopHook(executor.Start, executor.Stop))
		}),
	)
}

// ModuleWithNamespace creates an executor module for a specific namespace
func ModuleWithNamespace[SP ShardProcessor](namespace string) fx.Option {
	return fx.Module(fmt.Sprintf("shard-distributor-executor-client-%s", namespace),
		fx.Provide(func(params Params[SP]) (Executor[SP], error) {
			return NewExecutorWithNamespace(params, namespace)
		}),
		fx.Invoke(func(executor Executor[SP], lc fx.Lifecycle) {
			lc.Append(fx.StartStopHook(executor.Start, executor.Stop))
		}),
	)
}
