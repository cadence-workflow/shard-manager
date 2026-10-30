package metrics

import (
	"time"

	"github.com/uber-go/tally"
)

const (
	// Counter metrics
	CanaryPingSuccess           = "canary_ping_success"
	CanaryPingFailure           = "canary_ping_failure"
	CanaryPingOwnershipMismatch = "canary_ping_ownership_mismatch"
	CanaryShardCreated          = "canary_shard_created"
	CanaryShardStarted          = "canary_shard_started"
	CanaryShardStopped          = "canary_shard_stopped"
	CanaryShardDone             = "canary_shard_done"
	CanaryShardProcessStep      = "canary_shard_process_step"
	// CanaryShardLifecycleInjected counts every Start/Stop call where the
	// canary intentionally injected a delay. Tagged by lifecycle ("start" or
	// "stop") and bucket ("slow_start", "stuck_stop", ...). Use it to confirm
	// the canary is actually exercising the slow paths.
	CanaryShardLifecycleInjected = "canary_shard_lifecycle_injected"

	// Histogram metrics
	CanaryPingLatency = "canary_ping_latency"
	// CanaryShardStartLatency is the wall-clock duration of a shard
	// processor's Start() call, tagged by bucket. The "normal" bucket should
	// stay near zero; alert if it climbs.
	CanaryShardStartLatency = "canary_shard_start_latency"
	// CanaryShardStopLatency mirrors CanaryShardStartLatency for Stop().
	CanaryShardStopLatency = "canary_shard_stop_latency"

	// Load run metrics, tagged by namespace.
	CanaryLoadRunStarted         = "canary_load_run_started"
	CanaryLoadRunCompleted       = "canary_load_run_completed"
	CanaryLoadRunFailed          = "canary_load_run_failed"
	CanaryLoadRunDuration        = "canary_load_run_duration"
	CanaryLoadShardCreated       = "canary_load_shard_created"
	CanaryLoadShardCreateFailure = "canary_load_shard_create_failure"
	CanaryLoadShardCreateLatency = "canary_load_shard_create_latency"
	CanaryLoadExecutorsActive    = "canary_load_executors_active"
)

var (
	CanaryPingLatencyBuckets = tally.DurationBuckets([]time.Duration{
		1 * time.Millisecond,
		5 * time.Millisecond,
		10 * time.Millisecond,
		25 * time.Millisecond,
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
		5 * time.Second,
		10 * time.Second,
	})
)
