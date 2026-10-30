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
	"go.uber.org/mock/gomock"
	"go.uber.org/yarpc"
	"go.uber.org/zap/zaptest"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/config"
	canarymetrics "github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient"
)

const testNamespace = "test-load"

func testLoadConfig() config.LoadConfig {
	return config.LoadConfig{
		Namespace:       testNamespace,
		Interval:        time.Hour,
		NumShards:       3,
		ShardsPerSecond: 100,
		NumExecutors:    2,
		Hold:            10 * time.Second,
	}
}

func TestNewController(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*config.LoadConfig)
		wantErr string
	}{
		{name: "valid", modify: func(*config.LoadConfig) {}},
		{name: "zero rate", modify: func(c *config.LoadConfig) { c.ShardsPerSecond = 0 }, wantErr: "shards per second must be positive"},
		{name: "no executors", modify: func(c *config.LoadConfig) { c.NumExecutors = 0 }, wantErr: "number of executors must be positive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testLoadConfig()
			tt.modify(&cfg)

			_, err := NewController(Params{
				Config:       config.Config{Canary: config.CanaryConfig{Load: cfg}},
				Logger:       zaptest.NewLogger(t),
				TimeSource:   clock.NewMockedTimeSource(),
				MetricsScope: tally.NoopScope,
			})
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestRunOnce(t *testing.T) {
	notConfigured := &types.NamespaceNotFoundError{Namespace: testNamespace}
	shardNotFound := &types.ShardNotFoundError{Namespace: testNamespace, ShardKey: probeShardKey}

	tests := []struct {
		name string
		// setup sets expectations on the client; cancel stops the run as if the canary shut down.
		setup             func(client *sharddistributor.MockClient, cancel context.CancelFunc)
		failExecutorAt    int
		expectedExecutors int
		expectedCounters  map[string]int64
	}{
		{
			name: "namespace not configured skips the run",
			setup: func(client *sharddistributor.MockClient, _ context.CancelFunc) {
				client.EXPECT().InspectShard(gomock.Any(), gomock.Any()).Return(nil, notConfigured)
			},
			expectedCounters: map[string]int64{},
		},
		{
			name: "successful run",
			setup: func(client *sharddistributor.MockClient, _ context.CancelFunc) {
				client.EXPECT().InspectShard(gomock.Any(), gomock.Any()).Return(nil, shardNotFound)
				client.EXPECT().GetShardOwner(gomock.Any(), gomock.Any()).Return(&types.GetShardOwnerResponse{}, nil).Times(3)
			},
			expectedExecutors: 2,
			expectedCounters: map[string]int64{
				canarymetrics.CanaryLoadRunStarted:   1,
				canarymetrics.CanaryLoadRunCompleted: 1,
				canarymetrics.CanaryLoadShardCreated: 3,
			},
		},
		{
			name: "shard creation failures fail the run",
			setup: func(client *sharddistributor.MockClient, _ context.CancelFunc) {
				client.EXPECT().InspectShard(gomock.Any(), gomock.Any()).Return(nil, shardNotFound)
				client.EXPECT().GetShardOwner(gomock.Any(), gomock.Any()).Return(nil, errors.New("unavailable")).Times(3)
			},
			expectedExecutors: 2,
			expectedCounters: map[string]int64{
				canarymetrics.CanaryLoadRunStarted:         1,
				canarymetrics.CanaryLoadRunFailed:          1,
				canarymetrics.CanaryLoadShardCreateFailure: 3,
			},
		},
		{
			name: "executor creation failure stops the started executors",
			setup: func(client *sharddistributor.MockClient, _ context.CancelFunc) {
				client.EXPECT().InspectShard(gomock.Any(), gomock.Any()).Return(nil, shardNotFound)
			},
			failExecutorAt:    2,
			expectedExecutors: 1,
			expectedCounters: map[string]int64{
				canarymetrics.CanaryLoadRunStarted: 1,
				canarymetrics.CanaryLoadRunFailed:  1,
			},
		},
		{
			name: "shutdown during the run still stops the executors",
			setup: func(client *sharddistributor.MockClient, cancel context.CancelFunc) {
				client.EXPECT().InspectShard(gomock.Any(), gomock.Any()).Return(nil, shardNotFound)
				var calls atomic.Int32
				client.EXPECT().GetShardOwner(gomock.Any(), gomock.Any()).DoAndReturn(
					func(context.Context, *types.GetShardOwnerRequest, ...yarpc.CallOption) (*types.GetShardOwnerResponse, error) {
						if calls.Add(1) == 3 {
							cancel()
						}
						return &types.GetShardOwnerResponse{}, nil
					},
				).Times(3)
			},
			expectedExecutors: 2,
			expectedCounters: map[string]int64{
				canarymetrics.CanaryLoadRunStarted:   1,
				canarymetrics.CanaryLoadRunFailed:    1,
				canarymetrics.CanaryLoadShardCreated: 3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := sharddistributor.NewMockClient(ctrl)
			timeSource := clock.NewMockedTimeSource()
			scope := tally.NewTestScope("", nil)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tt.setup(client, cancel)

			c, err := NewController(Params{
				Config:                 config.Config{Canary: config.CanaryConfig{Load: testLoadConfig()}},
				ShardDistributorClient: client,
				Logger:                 zaptest.NewLogger(t),
				TimeSource:             timeSource,
				MetricsScope:           scope,
			})
			require.NoError(t, err)

			created := 0
			c.newExecutor = func(executorclient.ShardProcessorFactory[*ShardProcessor]) (executorclient.Executor[*ShardProcessor], error) {
				created++
				if created == tt.failExecutorAt {
					return nil, errors.New("create failed")
				}
				executor := executorclient.NewMockExecutor[*ShardProcessor](ctrl)
				executor.EXPECT().Start(gomock.Any())
				executor.EXPECT().GetShardStatusReports().Return(nil).AnyTimes()
				executor.EXPECT().Stop()
				return executor, nil
			}

			runUntilDone(timeSource, func() { c.runOnce(ctx) })

			counters := scope.Snapshot().Counters()
			for _, name := range []string{
				canarymetrics.CanaryLoadRunStarted,
				canarymetrics.CanaryLoadRunCompleted,
				canarymetrics.CanaryLoadRunFailed,
				canarymetrics.CanaryLoadShardCreated,
				canarymetrics.CanaryLoadShardCreateFailure,
			} {
				var got int64
				if counter, ok := counters[name+"+namespace="+testNamespace]; ok {
					got = counter.Value()
				}
				assert.Equal(t, tt.expectedCounters[name], got, name)
			}

			if tt.expectedExecutors > 0 {
				gauge := scope.Snapshot().Gauges()[canarymetrics.CanaryLoadExecutorsActive+"+namespace="+testNamespace]
				require.NotNil(t, gauge)
				assert.Zero(t, gauge.Value(), "executors must be stopped after the run")
			}
		})
	}
}

func TestStartStop(t *testing.T) {
	c, err := NewController(Params{
		Config:       config.Config{Canary: config.CanaryConfig{Load: testLoadConfig()}},
		Logger:       zaptest.NewLogger(t),
		TimeSource:   clock.NewMockedTimeSource(),
		MetricsScope: tally.NoopScope,
	})
	require.NoError(t, err)

	c.Start()
	c.Stop()
}

func TestShardProcessorReportsDoneAfterRelease(t *testing.T) {
	factory := &shardProcessorFactory{released: new(atomic.Bool)}
	processor, err := factory.NewShardProcessor("shard")
	require.NoError(t, err)

	assert.Equal(t, types.ShardStatusREADY, processor.GetShardReport().Status)
	factory.released.Store(true)
	assert.Equal(t, types.ShardStatusDONE, processor.GetShardReport().Status)
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
