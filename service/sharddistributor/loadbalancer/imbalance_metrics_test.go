package loadbalancer

import (
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

const testNamespace = "test-namespace"

func TestEmitAssignmentImbalanceMetrics_NaiveEmitsReportedLoadOnly(t *testing.T) {
	cfg := loadBalancingModeConfig(config.LoadBalancingModeNAIVE)
	ctrl := gomock.NewController(t)
	metricsScope := metrics.NewMockScope(ctrl)

	metricsScope.EXPECT().UpdateGauge(metrics.ShardDistributorAssignmentLoadMaxOverMean, 1.5).Times(1)
	metricsScope.EXPECT().UpdateGauge(metrics.ShardDistributorAssignmentLoadCV, 0.5).Times(1)

	EmitAssignmentImbalanceMetrics(cfg, testNamespace, metricsScope, testAssignments(), testNamespaceState(time.Now()))
}

// Greedy balancing uses persisted smoothed loads, so it emits both imbalance
// metrics and the ratio of assigned shards missing smoothed-load data.
func TestEmitAssignmentImbalanceMetrics_GreedyEmitsSmoothedLoadMetrics(t *testing.T) {
	cfg := loadBalancingModeConfig(config.LoadBalancingModeGREEDY)
	ctrl := gomock.NewController(t)
	metricsScope := metrics.NewMockScope(ctrl)

	metricsScope.EXPECT().UpdateGauge(metrics.ShardDistributorAssignmentLoadMaxOverMean, 1.5).Times(1)
	metricsScope.EXPECT().UpdateGauge(metrics.ShardDistributorAssignmentLoadCV, 0.5).Times(1)
	metricsScope.EXPECT().UpdateGauge(metrics.ShardDistributorAssignmentSmoothedLoadMaxOverMean, 1.5).Times(1)
	metricsScope.EXPECT().UpdateGauge(metrics.ShardDistributorAssignmentSmoothedLoadCV, 0.5).Times(1)
	metricsScope.EXPECT().UpdateGauge(metrics.ShardDistributorAssignmentSmoothedLoadMissingRatio, 1.0/3.0).Times(1)

	EmitAssignmentImbalanceMetrics(cfg, testNamespace, metricsScope, testAssignments(), testNamespaceState(time.Now()))
}

func loadBalancingModeConfig(mode string) *config.Config {
	return &config.Config{
		LoadBalancingMode: func(namespace string) string {
			return mode
		},
	}
}

func testAssignments() map[string][]string {
	return map[string][]string{
		"exec-1": {"shard-1", "shard-2"},
		"exec-2": {"shard-3"},
	}
}

func testNamespaceState(now time.Time) *store.NamespaceState {
	return &store.NamespaceState{
		Executors: map[string]store.HeartbeatState{
			"exec-1": {
				ReportedShards: map[string]*types.ShardStatusReport{
					"shard-1": {ShardLoad: 10},
					"shard-2": {ShardLoad: 20},
				},
			},
			"exec-2": {
				ReportedShards: map[string]*types.ShardStatusReport{
					"shard-3": {ShardLoad: 10},
				},
			},
		},
		ShardStats: map[string]store.ShardStatistics{
			"shard-1": {SmoothedLoad: 30, LastUpdateTime: now},
			// shard-2 is intentionally missing to exercise the missing-ratio metric.
			"shard-3": {SmoothedLoad: 10, LastUpdateTime: now},
		},
	}
}
