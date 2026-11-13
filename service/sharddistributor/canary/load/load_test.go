package load

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/mock/gomock"
	"go.uber.org/yarpc"
	"go.uber.org/zap/zaptest"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/config"
	canarymetrics "github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/clientcommon"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient"
)

const testNamespace = "test-load"

func testLoadConfig() config.LoadConfig {
	return config.LoadConfig{
		Namespace:          testNamespace,
		Interval:           time.Hour,
		BaselineExecutors:  1,
		NumExecutors:       2,
		ExecutorsPerSecond: 10,
		Hold:               10 * time.Second,
		ConvergenceTimeout: 10 * time.Second,
	}
}

func newTestController(t *testing.T, timeSource clock.TimeSource, scope tally.Scope) *Controller {
	c, err := NewController(Params{
		Logger:       zaptest.NewLogger(t),
		TimeSource:   timeSource,
		MetricsScope: scope,
	}, testLoadConfig())
	require.NoError(t, err)
	return c
}

func TestNewController(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*config.LoadConfig)
		wantErr string
	}{
		{name: "valid", modify: func(*config.LoadConfig) {}},
		{name: "invalid config", modify: func(c *config.LoadConfig) { c.NumExecutors = 0 }, wantErr: "invalid load config"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testLoadConfig()
			tt.modify(&cfg)

			_, err := NewController(Params{
				Logger:       zaptest.NewLogger(t),
				TimeSource:   clock.NewMockedTimeSource(),
				MetricsScope: tally.NoopScope,
			}, cfg)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestModule(t *testing.T) {
	tests := []struct {
		name           string
		namespace      string
		wantController bool
	}{
		{name: "empty namespace disables the load run", namespace: ""},
		{name: "namespace enables the load run", namespace: testNamespace, wantController: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			cfg := testLoadConfig()
			cfg.Namespace = tt.namespace

			var controller *Controller
			fxtest.New(t,
				fx.Supply(
					fx.Annotate(sharddistributor.NewMockClient(ctrl), fx.As(new(sharddistributor.Client))),
					fx.Annotate(executorclient.NewMockClient(ctrl), fx.As(new(executorclient.Client))),
					fx.Annotate(clock.NewMockedTimeSource(), fx.As(new(clock.TimeSource))),
					fx.Annotate(tally.NoopScope, fx.As(new(tally.Scope))),
					clientcommon.Config{},
					zaptest.NewLogger(t),
				),
				Module(cfg),
				fx.Invoke(func(p struct {
					fx.In
					Controller *Controller `optional:"true"`
				}) {
					controller = p.Controller
				}),
			)

			assert.Equal(t, tt.wantController, controller != nil)
		})
	}
}

func TestStartStop(t *testing.T) {
	tests := []struct {
		name    string
		failAt  int
		wantErr string
	}{
		{name: "baseline executors run until stop"},
		{name: "baseline executor creation failure", failAt: 2, wantErr: "create baseline load executor"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			c := newTestController(t, clock.NewMockedTimeSource(), tally.NoopScope)
			c.cfg.BaselineExecutors = 2

			created := 0
			c.newExecutor = func() (executorclient.Executor[*shardProcessor], error) {
				created++
				if created == tt.failAt {
					return nil, errors.New("create failed")
				}
				executor := executorclient.NewMockExecutor[*shardProcessor](ctrl)
				if tt.wantErr == "" {
					executor.EXPECT().Start(gomock.Any())
					executor.EXPECT().Stop()
				}
				return executor, nil
			}

			err := c.Start()
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			c.Stop()
		})
	}
}

func TestShardHolders(t *testing.T) {
	ctrl := gomock.NewController(t)
	newExecutor := func(id string, shards ...string) executorclient.Executor[*shardProcessor] {
		reports := make(map[string]*types.ShardStatusReport, len(shards))
		for _, shard := range shards {
			reports[shard] = &types.ShardStatusReport{Status: types.ShardStatusREADY}
		}
		executor := executorclient.NewMockExecutor[*shardProcessor](ctrl)
		executor.EXPECT().GetExecutorID().Return(id)
		executor.EXPECT().GetShardStatusReports().Return(reports)
		return executor
	}

	c := newTestController(t, clock.NewMockedTimeSource(), tally.NoopScope)
	c.baseline = []executorclient.Executor[*shardProcessor]{newExecutor("baseline", "0", "1")}
	c.extraExecutors = []executorclient.Executor[*shardProcessor]{newExecutor("extra", "2"), newExecutor("idle")}

	assert.ElementsMatch(t, []shardHolder{
		{shardKey: "0", executorID: "baseline"},
		{shardKey: "1", executorID: "baseline"},
		{shardKey: "2", executorID: "extra"},
	}, c.shardHolders())
}

func TestRunOnce(t *testing.T) {
	tests := []struct {
		name   string
		failAt int
		// cancelAt cancels the run when that extra executor starts, as if the canary shut down.
		cancelAt int
		// drained drains the switcher shard before the run; drainAt drains it when that extra executor starts.
		drained          bool
		drainAt          int
		expectedCreated  int
		expectedCounters map[string]int64
	}{
		{
			name:             "drained switcher shard skips the run",
			drained:          true,
			expectedCounters: map[string]int64{canarymetrics.CanaryLoadRunPaused: 1},
		},
		{
			name:            "successful run",
			expectedCreated: 2,
			expectedCounters: map[string]int64{
				canarymetrics.CanaryLoadRunStarted:   1,
				canarymetrics.CanaryLoadRunCompleted: 1,
			},
		},
		{
			name:            "draining the switcher shard during scale-up winds the run down",
			drainAt:         1,
			expectedCreated: 1,
			expectedCounters: map[string]int64{
				canarymetrics.CanaryLoadRunStarted:   1,
				canarymetrics.CanaryLoadRunPaused:    1,
				canarymetrics.CanaryLoadRunCompleted: 1,
			},
		},
		{
			name:            "draining the switcher shard during the hold winds the run down",
			drainAt:         2,
			expectedCreated: 2,
			expectedCounters: map[string]int64{
				canarymetrics.CanaryLoadRunStarted:   1,
				canarymetrics.CanaryLoadRunPaused:    1,
				canarymetrics.CanaryLoadRunCompleted: 1,
			},
		},
		{
			name:            "executor creation failure stops the started executors",
			failAt:          2,
			expectedCreated: 1,
			expectedCounters: map[string]int64{
				canarymetrics.CanaryLoadRunStarted: 1,
				canarymetrics.CanaryLoadRunFailed:  1,
			},
		},
		{
			name:            "shutdown during the run still stops the executors",
			cancelAt:        2,
			expectedCreated: 2,
			expectedCounters: map[string]int64{
				canarymetrics.CanaryLoadRunStarted: 1,
				canarymetrics.CanaryLoadRunFailed:  1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			timeSource := clock.NewMockedTimeSource()
			scope := tally.NewTestScope("", nil)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var drained atomic.Bool
			drained.Store(tt.drained)
			client := sharddistributor.NewMockClient(ctrl)
			client.EXPECT().GetShardOwner(gomock.Any(), &types.GetShardOwnerRequest{Namespace: testNamespace, ShardKey: switcherShardID}).DoAndReturn(
				func(context.Context, *types.GetShardOwnerRequest, ...yarpc.CallOption) (*types.GetShardOwnerResponse, error) {
					if !drained.Load() {
						return &types.GetShardOwnerResponse{}, nil
					}
					return nil, &types.ShardDrainedError{Namespace: testNamespace, ShardKey: switcherShardID}
				},
			).AnyTimes()
			// The test executors hold no shards, so any namespace state has converged
			client.EXPECT().GetNamespaceStateIfNewer(gomock.Any(), gomock.Any()).Return(&types.GetNamespaceStateIfNewerResponse{}, nil).AnyTimes()

			c := newTestController(t, timeSource, scope)
			c.client = client
			c.convergence.client = client
			if tt.drainAt > 0 {
				// A run that ignored the drain would hold far longer than the elapsed time asserted below
				c.cfg.Hold = time.Hour
			}

			calls, created := 0, 0
			c.newExecutor = func() (executorclient.Executor[*shardProcessor], error) {
				calls++
				if calls == tt.failAt {
					return nil, errors.New("create failed")
				}
				created++
				executor := executorclient.NewMockExecutor[*shardProcessor](ctrl)
				executor.EXPECT().GetExecutorID().Return("executor").AnyTimes()
				executor.EXPECT().GetShardStatusReports().Return(nil).AnyTimes()
				start := executor.EXPECT().Start(gomock.Any())
				switch calls {
				case tt.cancelAt:
					start.Do(func(context.Context) { cancel() })
				case tt.drainAt:
					start.Do(func(context.Context) { drained.Store(true) })
				}
				executor.EXPECT().Stop()
				return executor, nil
			}

			start := timeSource.Now()
			runUntilDone(timeSource, func() { c.runOnce(ctx) })
			if tt.drainAt > 0 {
				assert.Less(t, timeSource.Since(start), c.cfg.Hold, "the run must not wait out the hold once drained")
			}

			assert.Equal(t, tt.expectedCreated, created)
			assert.Empty(t, c.extraExecutors)
			counters := scope.Snapshot().Counters()
			for _, name := range []string{
				canarymetrics.CanaryLoadRunStarted,
				canarymetrics.CanaryLoadRunPaused,
				canarymetrics.CanaryLoadRunCompleted,
				canarymetrics.CanaryLoadRunFailed,
			} {
				var got int64
				if counter, ok := counters[name+"+namespace="+testNamespace]; ok {
					got = counter.Value()
				}
				assert.Equal(t, tt.expectedCounters[name], got, name)
			}

			if tt.expectedCreated > 0 {
				gauge := scope.Snapshot().Gauges()[canarymetrics.CanaryLoadExecutorsActive+"+namespace="+testNamespace]
				require.NotNil(t, gauge)
				assert.Zero(t, gauge.Value(), "executors must be stopped after the run")
			}
		})
	}
}

// runUntilDone runs f and keeps advancing the mocked clock until f returns.
func runUntilDone(timeSource clock.MockedTimeSource, f func()) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()

	for {
		select {
		case <-done:
			return
		case <-time.After(time.Millisecond):
			timeSource.Advance(time.Second)
		}
	}
}
