// Package load runs a periodic load test against a dedicated fixed canary namespace.
//
// Baseline executors keep the namespace's shards owned. Each run starts extra
// executors at a fixed rate, holds the peak, and then stops them at the same
// rate. Every join and leave makes the shard distributor reassign the
// namespace's shards, which surfaces how refresh, store and etcd latency
// behave as executor count and churn grow.
//
// After scaling up and after scaling down, the run waits for the shard
// distributor's cache to converge, see convergenceChecker.
//
// Draining switcherShardID pauses the load: no new run starts, and a run in
// progress skips the rest of its scale-up, hold and convergence checks and
// scales down at the usual rate.
package load

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/uber-go/tally"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/backoff"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/config"
	canarymetrics "github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/clientcommon"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient"
)

const (
	intervalJitterCoeff = 0.1
	pollJitterCoeff     = 0.5
	pollInterval        = time.Second

	phaseScaleUp   = "scale_up"
	phaseScaleDown = "scale_down"

	// switcherShardID is the load namespace shard that pauses the load while drained
	switcherShardID = "0"
)

// Params are the dependencies of the load Controller.
type Params struct {
	fx.In

	ShardDistributorClient sharddistributor.Client
	ExecutorClient         executorclient.Client
	Config                 clientcommon.Config
	Metadata               executorclient.ExecutorMetadata `optional:"true"`
	Logger                 *zap.Logger
	TimeSource             clock.TimeSource
	MetricsScope           tally.Scope
}

type loadExecutor = executorclient.Executor[*shardProcessor]

// Controller keeps the baseline executors running and starts one load run per interval
type Controller struct {
	cfg         config.LoadConfig
	client      sharddistributor.Client
	newExecutor func() (loadExecutor, error)
	convergence *convergenceChecker
	logger      *zap.Logger
	timeSource  clock.TimeSource
	scope       tally.Scope

	baseline []loadExecutor

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewController creates a Controller that runs load against cfg.Namespace.
func NewController(params Params, cfg config.LoadConfig) (*Controller, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid load config: %w", err)
	}

	logger := params.Logger.With(zap.String("namespace", cfg.Namespace))
	scope := params.MetricsScope.Tagged(map[string]string{"namespace": cfg.Namespace})
	return &Controller{
		cfg:    cfg,
		client: params.ShardDistributorClient,
		newExecutor: func() (loadExecutor, error) {
			return executorclient.NewExecutorWithNamespace(executorclient.Params[*shardProcessor]{
				ExecutorClient:        params.ExecutorClient,
				MetricsScope:          params.MetricsScope,
				Logger:                params.Logger,
				ShardProcessorFactory: &shardProcessorFactory{},
				Config:                params.Config,
				TimeSource:            params.TimeSource,
				Metadata:              params.Metadata,
			}, cfg.Namespace)
		},
		convergence: &convergenceChecker{
			client:     params.ShardDistributorClient,
			namespace:  cfg.Namespace,
			timeout:    cfg.ConvergenceTimeout,
			logger:     logger,
			timeSource: params.TimeSource,
			scope:      scope,
		},
		logger:     logger,
		timeSource: params.TimeSource,
		scope:      scope,
	}, nil
}

// Module provides the load Controller and starts it
func Module(cfg config.LoadConfig) fx.Option {
	if cfg.Namespace == "" {
		return fx.Options()
	}

	return fx.Module("canary-load",
		fx.Provide(func(params Params) (*Controller, error) {
			return NewController(params, cfg)
		}),
		fx.Invoke(func(lc fx.Lifecycle, c *Controller) {
			lc.Append(fx.StartStopHook(c.Start, c.Stop))
		}),
	)
}

// Start starts the baseline executors and the run loop
func (c *Controller) Start() error {
	baseline := make([]loadExecutor, 0, c.cfg.BaselineExecutors)
	for range c.cfg.BaselineExecutors {
		executor, err := c.newExecutor()
		if err != nil {
			return fmt.Errorf("create baseline load executor: %w", err)
		}
		baseline = append(baseline, executor)
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	for _, executor := range baseline {
		executor.Start(ctx)
	}
	c.baseline = baseline

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.loop(ctx)
	}()
	return nil
}

// Stop cancels any in-progress run, waits for its cleanup and stops the
// baseline executors
func (c *Controller) Stop() {
	c.cancel()
	c.wg.Wait()
	stopExecutors(c.baseline)
}

func (c *Controller) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.timeSource.After(backoff.JitDuration(c.cfg.Interval, intervalJitterCoeff)):
			c.runOnce(ctx)
		}
	}
}

func (c *Controller) runOnce(ctx context.Context) {
	if c.isPaused(ctx) {
		c.scope.Counter(canarymetrics.CanaryLoadRunPaused).Inc(1)
		c.logger.Info("load run skipped, the switcher shard is drained")
		return
	}

	c.scope.Counter(canarymetrics.CanaryLoadRunStarted).Inc(1)
	c.logger.Info("load run started")
	start := c.timeSource.Now()
	err := c.run(ctx)
	c.scope.Histogram(canarymetrics.CanaryLoadRunDuration, canarymetrics.CanaryLoadDurationBuckets).RecordDuration(c.timeSource.Since(start))

	if err != nil {
		c.scope.Counter(canarymetrics.CanaryLoadRunFailed).Inc(1)
		c.logger.Error("load run failed", zap.Error(err))
		return
	}
	c.scope.Counter(canarymetrics.CanaryLoadRunCompleted).Inc(1)
	c.logger.Info("load run completed")
}

// run scales the extra executors up and back down.
func (c *Controller) run(ctx context.Context) error {
	var extra []loadExecutor
	defer func() {
		stopExecutors(extra)
		c.reportActiveExecutors(0)
	}()

	extra, paused, err := c.scaleUp(ctx)
	if err != nil {
		return err
	}
	// A drained switcher shard cuts the scale-up short, the run then scales down straight away
	if !paused {
		if err := c.convergence.await(ctx, phaseScaleUp, slices.Concat(c.baseline, extra)); err != nil {
			return err
		}
		if err := c.hold(ctx); err != nil {
			return err
		}
	}
	if extra, err = c.scaleDown(ctx, extra); err != nil {
		return err
	}
	return c.convergence.await(ctx, phaseScaleDown, c.baseline)
}

// scaleUp starts cfg.NumExecutors extra executors at cfg.ExecutorsPerSecond.
// It returns the executors it started, even on error, and whether it stopped
// early because the switcher shard is drained.
func (c *Controller) scaleUp(ctx context.Context) (extra []loadExecutor, paused bool, err error) {
	extra = make([]loadExecutor, 0, c.cfg.NumExecutors)
	for range c.cfg.NumExecutors {
		if err := c.timeSource.SleepWithContext(ctx, c.executorStep()); err != nil {
			return extra, false, err
		}
		if c.windingDown(ctx) {
			return extra, true, nil
		}
		executor, err := c.newExecutor()
		if err != nil {
			return extra, false, fmt.Errorf("create load executor: %w", err)
		}
		executor.Start(ctx)
		extra = append(extra, executor)
		c.reportActiveExecutors(len(extra))
	}
	return extra, false, nil
}

// scaleDown stops the extra executors at cfg.ExecutorsPerSecond
// most recently started first, and returns the ones still running.
func (c *Controller) scaleDown(ctx context.Context, extra []loadExecutor) ([]loadExecutor, error) {
	for len(extra) > 0 {
		if err := c.timeSource.SleepWithContext(ctx, c.executorStep()); err != nil {
			return extra, err
		}
		last := len(extra) - 1
		extra[last].Stop()
		extra = extra[:last]
		c.reportActiveExecutors(len(extra))
	}
	return extra, nil
}

// executorStep is the wait before each executor start or stop
func (c *Controller) executorStep() time.Duration {
	return time.Second / time.Duration(c.cfg.ExecutorsPerSecond)
}

// hold waits for cfg.Hold, returning early once the switcher shard is drained
func (c *Controller) hold(ctx context.Context) error {
	deadline := c.timeSource.After(c.cfg.Hold)
	ticker := c.timeSource.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return nil
		case <-ticker.Chan():
			if c.windingDown(ctx) {
				return nil
			}
		}
	}
}

// windingDown reports whether the switcher shard is drained, recording that
// the run is cutting its load short
func (c *Controller) windingDown(ctx context.Context) bool {
	if !c.isPaused(ctx) {
		return false
	}
	c.scope.Counter(canarymetrics.CanaryLoadRunPaused).Inc(1)
	c.logger.Info("load run winding down, the switcher shard is drained")
	return true
}

// isPaused reports whether switcherShardID is drained
func (c *Controller) isPaused(ctx context.Context) bool {
	_, err := c.client.GetShardOwner(ctx, &types.GetShardOwnerRequest{Namespace: c.cfg.Namespace, ShardKey: switcherShardID})
	var drained *types.ShardDrainedError
	return errors.As(err, &drained)
}

func (c *Controller) reportActiveExecutors(active int) {
	c.scope.Gauge(canarymetrics.CanaryLoadExecutorsActive).Update(float64(active))
}

func stopExecutors(executors []loadExecutor) {
	var wg sync.WaitGroup
	for _, executor := range executors {
		wg.Go(executor.Stop)
	}
	wg.Wait()
}

// shardProcessor is a load shard that does no work and always reports READY.
type shardProcessor struct{}

var _ executorclient.ShardProcessor = (*shardProcessor)(nil)

func (p *shardProcessor) Start(context.Context) error { return nil }

func (p *shardProcessor) Stop() {}

func (p *shardProcessor) GetShardReport() executorclient.ShardReport {
	return executorclient.ShardReport{ShardLoad: 1.0, Status: types.ShardStatusREADY}
}

func (p *shardProcessor) SetShardStatus(types.ShardStatus) {}

type shardProcessorFactory struct{}

func (f *shardProcessorFactory) NewShardProcessor(string) (*shardProcessor, error) {
	return &shardProcessor{}, nil
}
