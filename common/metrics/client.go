package metrics

import (
	"time"

	"github.com/uber-go/tally"
)

// ClientImpl is used for reporting metrics by various Cadence services
type ClientImpl struct {
	// parentReporter is the parent scope for the metrics
	parentScope     tally.Scope
	childScopes     map[ScopeIdx]tally.Scope
	metricDefs      map[MetricIdx]metricDefinition
	serviceIdx      ServiceIdx
	migrationConfig MigrationConfig
}

// NewClient creates and returns a new instance of
// Client implementation
// reporter holds the common tags for the service
// serviceIdx indicates the service type in (InputhostIndex, ... StorageIndex)
func NewClient(scope tally.Scope, serviceIdx ServiceIdx, mc MigrationConfig) Client {
	totalScopes := len(ScopeDefs[Common]) + len(ScopeDefs[serviceIdx])
	metricsClient := &ClientImpl{
		parentScope:     scope,
		childScopes:     make(map[ScopeIdx]tally.Scope, totalScopes),
		metricDefs:      getMetricDefs(serviceIdx),
		serviceIdx:      serviceIdx,
		migrationConfig: mc,
	}

	for idx, def := range ScopeDefs[Common] {
		scopeTags := map[string]string{
			OperationTagName: def.operation,
		}
		mergeMapToRight(def.tags, scopeTags)
		metricsClient.childScopes[idx] = scope.Tagged(scopeTags)
	}

	for idx, def := range ScopeDefs[serviceIdx] {
		scopeTags := map[string]string{
			OperationTagName: def.operation,
		}
		mergeMapToRight(def.tags, scopeTags)
		metricsClient.childScopes[idx] = scope.Tagged(scopeTags)
	}

	return metricsClient
}

// IncCounter increments one for a counter and emits
// to metrics backend
func (m *ClientImpl) IncCounter(scope ScopeIdx, counterIdx MetricIdx) {
	def := m.metricDefs[counterIdx]
	name := string(def.metricName)
	if m.migrationConfig.Counter.EmitCounter(name) {
		m.childScopes[scope].Counter(name).Inc(1)
	}
}

// AddCounter adds delta to the counter and
// emits to the metrics backend
func (m *ClientImpl) AddCounter(scope ScopeIdx, counterIdx MetricIdx, delta int64) {
	def := m.metricDefs[counterIdx]
	name := string(def.metricName)
	if m.migrationConfig.Counter.EmitCounter(name) {
		m.childScopes[scope].Counter(name).Inc(delta)
	}
}

// StartTimer starts a timer for the given
// metric name
func (m *ClientImpl) StartTimer(scope ScopeIdx, timerIdx MetricIdx) tally.Stopwatch {
	name := string(m.metricDefs[timerIdx].metricName)
	if m.migrationConfig.Histogram.EmitTimer(name) {
		return m.childScopes[scope].Timer(name).Start()
	}
	return NoopStopwatch
}

// RecordTimer record and emit a timer for the given
// metric name
func (m *ClientImpl) RecordTimer(scope ScopeIdx, timerIdx MetricIdx, d time.Duration) {
	name := string(m.metricDefs[timerIdx].metricName)
	if m.migrationConfig.Histogram.EmitTimer(name) {
		m.childScopes[scope].Timer(name).Record(d)
	}
}

// RecordHistogramDuration record and emit a duration
func (m *ClientImpl) RecordHistogramDuration(scope ScopeIdx, timerIdx MetricIdx, d time.Duration) {
	name := string(m.metricDefs[timerIdx].metricName)
	if m.migrationConfig.Histogram.EmitHistogram(name) {
		m.childScopes[scope].Histogram(name, m.getBuckets(timerIdx)).RecordDuration(d)
	}
}

// UpdateGauge reports Gauge type metric
func (m *ClientImpl) UpdateGauge(scopeIdx ScopeIdx, gaugeIdx MetricIdx, value float64) {
	def := m.metricDefs[gaugeIdx]
	name := string(def.metricName)
	if m.migrationConfig.Gauge.EmitGauge(name) {
		m.childScopes[scopeIdx].Gauge(name).Update(value)
	}
}

// Scope return a new internal metrics scope that can be used to add additional
// information to the metrics emitted
func (m *ClientImpl) Scope(scope ScopeIdx, tags ...Tag) Scope {
	sc := m.childScopes[scope]
	return newMetricsScope(sc, sc, m.metricDefs, false, m.migrationConfig).Tagged(tags...)
}

func (m *ClientImpl) getBuckets(id MetricIdx) tally.Buckets {
	if m.metricDefs[id].buckets != nil {
		return m.metricDefs[id].buckets
	}
	return tally.DefaultBuckets
}

func getMetricDefs(serviceIdx ServiceIdx) map[MetricIdx]metricDefinition {
	defs := make(map[MetricIdx]metricDefinition)
	for idx, def := range MetricDefs[Common] {
		defs[idx] = def
	}

	for idx, def := range MetricDefs[serviceIdx] {
		defs[idx] = def
	}

	return defs
}

func mergeMapToRight(src map[string]string, dest map[string]string) {
	for k, v := range src {
		dest[k] = v
	}
}
