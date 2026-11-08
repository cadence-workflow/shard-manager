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
// progress skips the rest of its scale-up and hold and scales down at the
// usual rate.
package load

import (
	"context"
	"errors"
	"fmt"
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
	intervalJitterCoeff   = 0.1
	maxExecutorsPerSecond = 100
	pollInterval          = time.Second

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

// Controller keeps the baseline executors running and starts one load run per interval
type Controller struct {
	cfg         config.LoadConfig
	client      sharddistributor.Client
	newExecutor func() (executorclient.Executor[*shardProcessor], error)
	convergence *convergenceChecker
	logger      *zap.Logger
	timeSource  clock.TimeSource
	scope       tally.Scope

	mu        sync.RWMutex
	baseline  []executorclient.Executor[*shardProcessor]
	executors []executorclient.Executor[*shardProcessor]

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewController creates a Controller that runs load against cfg.Namespace.
func NewController(params Params, cfg config.LoadConfig) (*Controller, error) {
	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid load config: %w", err)
	}

	logger := params.Logger.With(zap.String("namespace", cfg.Namespace))
	scope := params.MetricsScope.Tagged(map[string]string{"namespace": cfg.Namespace})
	c := &Controller{
		cfg:    cfg,
		client: params.ShardDistributorClient,
		newExecutor: func() (executorclient.Executor[*shardProcessor], error) {
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
		logger:     logger,
		timeSource: params.TimeSource,
		scope:      scope,
	}
	c.convergence = &convergenceChecker{
		client:     params.ShardDistributorClient,
		namespace:  cfg.Namespace,
		holders:    c.shardHolders,
		timeout:    cfg.ConvergenceTimeout,
		logger:     logger,
		timeSource: params.TimeSource,
		scope:      scope,
	}
	return c, nil
}

func validate(cfg config.LoadConfig) error {
	switch {
	case cfg.Namespace == "":
		return errors.New("namespace must be set")
	case cfg.NumShards <= 0:
		return errors.New("number of shards must be positive")
	case cfg.Interval <= 0:
		return errors.New("interval must be positive")
	case cfg.BaselineExecutors <= 0:
		return errors.New("number of baseline executors must be positive")
	case cfg.NumExecutors <= 0:
		return errors.New("number of executors must be positive")
	case cfg.ExecutorsPerSecond <= 0 || cfg.ExecutorsPerSecond > maxExecutorsPerSecond:
		return fmt.Errorf("executors per second must be in (0, %d]", maxExecutorsPerSecond)
	case cfg.Hold < 0:
		return errors.New("hold must not be negative")
	case cfg.ConvergenceTimeout <= 0:
		return errors.New("convergence timeout must be positive")
	}
	return nil
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
	baseline := make([]executorclient.Executor[*shardProcessor], 0, c.cfg.BaselineExecutors)
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
	c.mu.Lock()
	c.baseline = baseline
	c.mu.Unlock()

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

	c.mu.Lock()
	baseline := c.baseline
	c.baseline = nil
	c.mu.Unlock()
	stopExecutors(baseline)
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
	c.scope.Timer(canarymetrics.CanaryLoadRunDuration).Record(c.timeSource.Since(start))

	if err != nil {
		c.scope.Counter(canarymetrics.CanaryLoadRunFailed).Inc(1)
		c.logger.Error("load run failed", zap.Error(err))
		return
	}
	c.scope.Counter(canarymetrics.CanaryLoadRunCompleted).Inc(1)
	c.logger.Info("load run completed")
}

func (c *Controller) run(ctx context.Context) error {
	// On errors and shutdown, the remaining extra executors stop at once.
	// The shard distributor reassigns their shards to the baseline executors
	defer func() {
		c.mu.Lock()
		executors := c.executors
		c.executors = nil
		c.mu.Unlock()
		stopExecutors(executors)
		c.scope.Gauge(canarymetrics.CanaryLoadExecutorsActive).Update(0)
	}()

	ticker := c.timeSource.NewTicker(time.Second / time.Duration(c.cfg.ExecutorsPerSecond))
	defer ticker.Stop()

	started, err := c.scaleUp(ctx, ticker)
	if err != nil {
		return err
	}
	// A drained switcher shard cuts the scale-up short, the run then scales down straight away
	if started == c.cfg.NumExecutors {
		if err := c.convergence.await(ctx, phaseScaleUp); err != nil {
			return err
		}
		if err := c.hold(ctx); err != nil {
			return err
		}
	}

	for range started {
		if err := waitTick(ctx, ticker); err != nil {
			return err
		}
		var stopped executorclient.Executor[*shardProcessor]
		c.updateExecutors(func(executors []executorclient.Executor[*shardProcessor]) []executorclient.Executor[*shardProcessor] {
			last := len(executors) - 1
			stopped = executors[last]
			return executors[:last]
		})
		stopped.Stop()
	}
	return c.convergence.await(ctx, phaseScaleDown)
}

// scaleUp starts up to cfg.NumExecutors executors, one per tick, and stops
// early once the switcher shard is drained
// It returns how many it started
func (c *Controller) scaleUp(ctx context.Context, ticker clock.Ticker) (int, error) {
	for started := range c.cfg.NumExecutors {
		if err := waitTick(ctx, ticker); err != nil {
			return started, err
		}
		if c.windingDown(ctx) {
			return started, nil
		}
		executor, err := c.newExecutor()
		if err != nil {
			return started, fmt.Errorf("create load executor: %w", err)
		}
		executor.Start(ctx)
		c.updateExecutors(func(executors []executorclient.Executor[*shardProcessor]) []executorclient.Executor[*shardProcessor] {
			return append(executors, executor)
		})
	}
	return c.cfg.NumExecutors, nil
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

func waitTick(ctx context.Context, ticker clock.Ticker) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ticker.Chan():
		return nil
	}
}

func (c *Controller) shardHolders() []shardHolder {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var holders []shardHolder
	for _, executors := range [][]executorclient.Executor[*shardProcessor]{c.baseline, c.executors} {
		for _, executor := range executors {
			executorID := executor.GetExecutorID()
			for shardKey := range executor.GetShardStatusReports() {
				holders = append(holders, shardHolder{shardKey: shardKey, executorID: executorID})
			}
		}
	}
	return holders
}

// isPaused reports whether switcherShardID is drained
func (c *Controller) isPaused(ctx context.Context) bool {
	_, err := c.client.GetShardOwner(ctx, &types.GetShardOwnerRequest{Namespace: c.cfg.Namespace, ShardKey: switcherShardID})
	var drained *types.ShardDrainedError
	return errors.As(err, &drained)
}

func (c *Controller) updateExecutors(update func([]executorclient.Executor[*shardProcessor]) []executorclient.Executor[*shardProcessor]) {
	c.mu.Lock()
	c.executors = update(c.executors)
	active := len(c.executors)
	c.mu.Unlock()
	c.scope.Gauge(canarymetrics.CanaryLoadExecutorsActive).Update(float64(active))
}

func stopExecutors(executors []executorclient.Executor[*shardProcessor]) {
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
