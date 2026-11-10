package prometheus

import (
	"time"

	"github.com/uber-go/tally/prometheus"
)

// DefaultHistogramBuckets is the default histogram buckets used when
// creating a new Histogram in the prometheus registry.
// Cadence uses Histogram to report both latency and non-latency metrics(like history_count/history_size).
// The buckets need to set correct boundary for both
// See more in https://github.com/cadence-workflow/shard-manager/issues/4006
func DefaultHistogramBuckets() []prometheus.HistogramObjective {

	// For latency metrics, this is a nanoSecond.
	// For non-latency metrics, this is ONE.
	// divided by Second because of https://github.com/uber-go/tally/blob/04828d51c63b1d09b46824d7c0d7904b5eb1b3b6/prometheus/reporter.go#L183
	oneOrNanoSecond := float64(time.Nanosecond) / float64(time.Second)

	// convenient units
	microSecOr1K := oneOrNanoSecond * 1000
	milliSecOr1M := oneOrNanoSecond * 1000000

	return []prometheus.HistogramObjective{
		{Upper: 5 * oneOrNanoSecond},
		{Upper: 10 * oneOrNanoSecond},
		{Upper: 50 * oneOrNanoSecond},
		{Upper: 100 * oneOrNanoSecond},

		{Upper: microSecOr1K},
		{Upper: 10 * microSecOr1K},
		{Upper: 50 * microSecOr1K},
		{Upper: 200 * microSecOr1K},

		// Below are exactly the same as https://github.com/uber-go/tally/blob/04828d51c63b1d09b46824d7c0d7904b5eb1b3b6/prometheus/reporter.go#L51
		// to ensure backward compatibility
		{Upper: milliSecOr1M},
		{Upper: 2 * milliSecOr1M},
		{Upper: 5 * milliSecOr1M},
		{Upper: 10 * milliSecOr1M},
		{Upper: 20 * milliSecOr1M},
		{Upper: 50 * milliSecOr1M},
		{Upper: 100 * milliSecOr1M},
		{Upper: 200 * milliSecOr1M},
		{Upper: 500 * milliSecOr1M},
		{Upper: 1000 * milliSecOr1M},
		{Upper: 2000 * milliSecOr1M},
		{Upper: 5000 * milliSecOr1M},
		{Upper: 10000 * milliSecOr1M},
	}
}
