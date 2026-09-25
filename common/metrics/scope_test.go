package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber-go/tally"
)

func TestHistogramMode(t *testing.T) {
	ts := tally.NewTestScope("", nil)
	findName := func(m MetricIdx) string {
		def, ok := MetricDefs[Common][m]
		if ok {
			return def.metricName.String()
		}
		def, ok = MetricDefs[ShardDistributor][m]
		if ok {
			return def.metricName.String()
		}
		t.Fatalf("MetricDef not found in common or shard distributor: %v", m)
		return "unknown"
	}

	orig := HistogramMigrationMetrics
	t.Cleanup(func() {
		HistogramMigrationMetrics = orig
	})

	HistogramMigrationMetrics = map[string]struct{}{
		findName(CadenceClientLatency):                   {},
		findName(ShardDistributorWatchProcessingLatency): {},
	}

	c := NewClient(ts, ShardDistributor, MigrationConfig{
		Histogram: HistogramMigration{
			Names: map[string]bool{
				findName(CadenceClientLatency):                   true,
				findName(ShardDistributorWatchProcessingLatency): true,
			},
		},
	})
	scope := c.Scope(ShardDistributorClientGetShardOwnerScope)

	scope.RecordTimer(CadenceClientLatency, time.Second)
	scope.RecordHistogramDuration(ShardDistributorWatchProcessingLatency, 2*time.Second)
	scope.RecordTimer(ShardDistributorLatency, 3*time.Second)

	s := ts.Snapshot()
	findMetric := func(idx MetricIdx) (timer, histogram bool) {
		name := findName(idx)
		for _, v := range s.Timers() {
			if v.Name() == name {
				t.Logf("found timer: %v = %v", v.Name(), v.Values())
				timer = true
				break
			}
		}
		for _, v := range s.Histograms() {
			if v.Name() == findName(idx) {
				nzDur := make(map[time.Duration]int64, 1)
				for k, val := range v.Durations() {
					if val != 0 {
						nzDur[k] = val
					}
				}
				nzVal := make(map[float64]int64, 1)
				for k, val := range v.Values() {
					if val != 0 {
						nzVal[k] = val
					}
				}
				t.Logf("found histogram: %v = %v (values: %v)", v.Name(), nzDur, nzVal)
				histogram = true
				break
			}
		}
		return
	}
	assertFound := func(idx MetricIdx, timer, histogram bool) {
		name := findName(idx)
		foundTimer, foundHistogram := findMetric(idx)
		assert.Equalf(t, foundTimer, timer, "wrong timer behavior for %v", name)
		assert.Equalf(t, foundHistogram, histogram, "wrong histogram behavior for %v", name)
	}

	assertFound(CadenceClientLatency, true, false)
	assertFound(ShardDistributorWatchProcessingLatency, false, true)
	assertFound(ShardDistributorLatency, true, false)
}
