// Package load runs a periodic load test against a dedicated canary namespace.
//
// Each run starts extra executors, creates shards through GetShardOwner at a
// fixed rate, holds the peak, and then releases the shards and stops the
// executors. It surfaces how shard distributor refresh, store and etcd latency
// behave as shard and executor count grow.
package load

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
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
	maxInFlightCreates  = 64
	heartbeatInterval   = time.Second
	releasePollInterval = time.Second
	releaseTimeout      = time.Minute
	probeShardKey       = "load-probe"
)

// Params are the dependencies of the load Controller.
type Params struct {
	fx.In

	Config                 config.Config
	ShardDistributorClient sharddistributor.Client
	ExecutorClient         executorclient.Client
	Logger                 *zap.Logger
	TimeSource             clock.TimeSource
	MetricsScope           tally.Scope
}

// Controller starts one load run per interval. Runs never overlap.
type Controller struct {
	cfg         config.LoadConfig
	client      sharddistributor.Client
	newExecutor func(executorclient.ShardProcessorFactory[*ShardProcessor]) (executorclient.Executor[*ShardProcessor], error)
	logger      *zap.Logger
	timeSource  clock.TimeSource
	scope       tally.Scope

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewController creates a Controller for the load namespace in params.Config.
func NewController(params Params) (*Controller, error) {
	cfg := params.Config.Canary.Load
	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid load config: %w", err)
	}

	// Load executors get their own config so the load namespace is not added
	// to the spectators and executors built from the shared canary config.
	executorConfig := clientcommon.Config{
		Namespaces: []clientcommon.NamespaceConfig{{Namespace: cfg.Namespace, HeartBeatInterval: heartbeatInterval}},
	}

	return &Controller{
		cfg:    cfg,
		client: params.ShardDistributorClient,
		newExecutor: func(factory executorclient.ShardProcessorFactory[*ShardProcessor]) (executorclient.Executor[*ShardProcessor], error) {
			return executorclient.NewExecutorWithNamespace(executorclient.Params[*ShardProcessor]{
				ExecutorClient:        params.ExecutorClient,
				MetricsScope:          params.MetricsScope,
				Logger:                params.Logger,
				ShardProcessorFactory: factory,
				Config:                executorConfig,
				TimeSource:            params.TimeSource,
				Enabled:               func() bool { return true },
			}, cfg.Namespace)
		},
		logger:     params.Logger.With(zap.String("namespace", cfg.Namespace)),
		timeSource: params.TimeSource,
		scope:      params.MetricsScope.Tagged(map[string]string{"namespace": cfg.Namespace}),
	}, nil
}

func validate(cfg config.LoadConfig) error {
	switch {
	case cfg.Namespace == "":
		return errors.New("namespace must be set")
	case cfg.Interval <= 0:
		return errors.New("interval must be positive")
	case cfg.NumShards <= 0:
		return errors.New("number of shards must be positive")
	case cfg.ShardsPerSecond <= 0:
		return errors.New("shards per second must be positive")
	case cfg.NumExecutors <= 0:
		return errors.New("number of executors must be positive")
	case cfg.Hold < 0:
		return errors.New("hold must not be negative")
	}
	return nil
}

// Module provides the load Controller and ties it to the fx lifecycle.
func Module() fx.Option {
	return fx.Module("canary-load",
		fx.Provide(NewController),
		fx.Invoke(func(lc fx.Lifecycle, c *Controller) {
			lc.Append(fx.StartStopHook(c.Start, c.Stop))
		}),
	)
}

// Start begins the run loop in a background goroutine.
func (c *Controller) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.loop(ctx)
	}()
}

// Stop cancels any in-progress run and waits for its cleanup to finish.
func (c *Controller) Stop() {
	c.cancel()
	c.wg.Wait()
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
	if !c.namespaceConfigured(ctx) {
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

// namespaceConfigured reports whether the shard distributor serves the load
// namespace. Environments opt in by adding the namespace to the shard
// distributor config, so an unknown namespace skips the run instead of failing it.
func (c *Controller) namespaceConfigured(ctx context.Context) bool {
	_, err := c.client.InspectShard(ctx, &types.GetShardOwnerRequest{Namespace: c.cfg.Namespace, ShardKey: probeShardKey})
	var notFound *types.NamespaceNotFoundError
	if errors.As(err, &notFound) {
		c.logger.Info("skipping load run, namespace is not configured on the shard distributor")
		return false
	}
	return true
}

func (c *Controller) run(ctx context.Context) error {
	released := &atomic.Bool{}
	executors, err := c.startExecutors(ctx, released)
	defer func() {
		released.Store(true)
		c.waitForRelease(ctx, executors)
		c.stopExecutors(executors)
	}()
	if err != nil {
		return err
	}

	failures := c.createShards(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.timeSource.After(c.cfg.Hold):
	}

	if failures > 0 {
		return fmt.Errorf("%d of %d shard creations failed", failures, c.cfg.NumShards)
	}
	return nil
}

func (c *Controller) startExecutors(ctx context.Context, released *atomic.Bool) ([]executorclient.Executor[*ShardProcessor], error) {
	factory := &shardProcessorFactory{released: released}
	executors := make([]executorclient.Executor[*ShardProcessor], 0, c.cfg.NumExecutors)
	defer func() {
		c.scope.Gauge(canarymetrics.CanaryLoadExecutorsActive).Update(float64(len(executors)))
	}()

	for range c.cfg.NumExecutors {
		executor, err := c.newExecutor(factory)
		if err != nil {
			return executors, fmt.Errorf("create load executor: %w", err)
		}
		executor.Start(ctx)
		executors = append(executors, executor)
	}
	return executors, nil
}

// createShards creates NumShards shard keys at ShardsPerSecond and returns
// how many creations failed.
func (c *Controller) createShards(ctx context.Context) int64 {
	ticker := c.timeSource.NewTicker(time.Second / time.Duration(c.cfg.ShardsPerSecond))
	defer ticker.Stop()

	var (
		wg       sync.WaitGroup
		failures atomic.Int64
		inFlight = make(chan struct{}, maxInFlightCreates)
	)

	for range c.cfg.NumShards {
		select {
		case <-ctx.Done():
			wg.Wait()
			return failures.Load()
		case <-ticker.Chan():
		}

		inFlight <- struct{}{}
		wg.Go(func() {
			defer func() { <-inFlight }()
			if err := c.createShard(ctx); err != nil {
				failures.Add(1)
			}
		})
	}

	wg.Wait()
	return failures.Load()
}

func (c *Controller) createShard(ctx context.Context) error {
	start := c.timeSource.Now()
	_, err := c.client.GetShardOwner(ctx, &types.GetShardOwnerRequest{Namespace: c.cfg.Namespace, ShardKey: uuid.New().String()})
	c.scope.Histogram(canarymetrics.CanaryLoadShardCreateLatency, canarymetrics.CanaryPingLatencyBuckets).
		RecordDuration(c.timeSource.Since(start))

	if err != nil {
		c.scope.Counter(canarymetrics.CanaryLoadShardCreateFailure).Inc(1)
		c.logger.Debug("failed to create load shard", zap.Error(err))
		return err
	}
	c.scope.Counter(canarymetrics.CanaryLoadShardCreated).Inc(1)
	return nil
}

// waitForRelease waits until the shard distributor has dropped the released
// shards from every executor, the timeout passes, or ctx is done.
func (c *Controller) waitForRelease(ctx context.Context, executors []executorclient.Executor[*ShardProcessor]) {
	timeout := c.timeSource.After(releaseTimeout)
	ticker := c.timeSource.NewTicker(releasePollInterval)
	defer ticker.Stop()

	for ownsShards(executors) {
		select {
		case <-ctx.Done():
			return
		case <-timeout:
			c.logger.Warn("load shards were not released before the timeout, stopping executors anyway")
			return
		case <-ticker.Chan():
		}
	}
}

func ownsShards(executors []executorclient.Executor[*ShardProcessor]) bool {
	for _, executor := range executors {
		if len(executor.GetShardStatusReports()) > 0 {
			return true
		}
	}
	return false
}

func (c *Controller) stopExecutors(executors []executorclient.Executor[*ShardProcessor]) {
	var wg sync.WaitGroup
	for _, executor := range executors {
		wg.Go(executor.Stop)
	}
	wg.Wait()
	c.scope.Gauge(canarymetrics.CanaryLoadExecutorsActive).Update(0)
}

// ShardProcessor is a load shard that does no work. It reports READY until
// its run is released and DONE afterwards, so the shard distributor drops it.
type ShardProcessor struct {
	released *atomic.Bool
}

var _ executorclient.ShardProcessor = (*ShardProcessor)(nil)

func (p *ShardProcessor) Start(context.Context) error { return nil }

func (p *ShardProcessor) Stop() {}

func (p *ShardProcessor) GetShardReport() executorclient.ShardReport {
	status := types.ShardStatusREADY
	if p.released.Load() {
		status = types.ShardStatusDONE
	}
	return executorclient.ShardReport{ShardLoad: 1.0, Status: status}
}

// SetShardStatus is a no-op: load shards finish only when their run is released.
func (p *ShardProcessor) SetShardStatus(types.ShardStatus) {}

type shardProcessorFactory struct {
	released *atomic.Bool
}

func (f *shardProcessorFactory) NewShardProcessor(string) (*ShardProcessor, error) {
	return &ShardProcessor{released: f.released}, nil
}
