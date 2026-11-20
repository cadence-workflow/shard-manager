package metered

import (
	"errors"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

type base struct {
	metricClient metrics.Client
	logger       log.Logger
	timeSource   clock.TimeSource
}

func (p *base) updateErrorMetricPerNamespace(err error, scopeWithNamespaceTags metrics.Scope, scopeIdx metrics.ScopeIdx) {
	if isShardStatisticsSkip(err, scopeIdx) {
		scopeWithNamespaceTags.IncCounter(metrics.ShardDistributorStoreShardStatisticsSkipped)
		return
	}

	if errors.Is(err, store.ErrExecutorNotFound) {
		scopeWithNamespaceTags.IncCounter(metrics.ShardDistributorStoreExecutorNotFound)
	}

	scopeWithNamespaceTags.IncCounter(metrics.ShardDistributorStoreFailuresPerNamespace)
}

func (p *base) call(scopeIdx metrics.ScopeIdx, op func() error, tags ...metrics.Tag) error {
	metricsScope := p.metricClient.Scope(scopeIdx, tags...)

	metricsScope.IncCounter(metrics.ShardDistributorStoreRequestsPerNamespace)
	before := p.timeSource.Now()
	err := op()
	duration := p.timeSource.Since(before)
	metricsScope.RecordHistogramDuration(metrics.ShardDistributorStoreLatencyHistogramPerNamespace, duration)

	if err != nil {
		p.updateErrorMetricPerNamespace(err, metricsScope, scopeIdx)
	}
	return err
}

func isShardStatisticsSkip(err error, scopeIdx metrics.ScopeIdx) bool {
	return scopeIdx == metrics.ShardDistributorStoreRecordShardStatisticsScope &&
		errors.Is(err, store.ErrVersionConflict)
}
