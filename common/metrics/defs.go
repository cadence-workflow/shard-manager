// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package metrics

import (
	"time"

	"github.com/uber-go/tally"
)

// types used/defined by the package
type (
	// MetricName is the name of the metric
	MetricName string

	// MetricType is the type of the metric
	MetricType int

	// metricDefinition contains the definition for a metric
	metricDefinition struct {
		metricType            MetricType    // metric type
		metricName            MetricName    // metric name
		metricRollupName      MetricName    // optional. if non-empty, this name must be used for rolled-up version of this metric
		buckets               tally.Buckets // buckets if we are emitting histograms
		exponentialBuckets    histogrammy[SubsettableHistogram]
		intExponentialBuckets histogrammy[IntSubsettableHistogram]
	}

	// scopeDefinition holds the tag definitions for a scope
	scopeDefinition struct {
		operation string            // 'operation' tag for scope
		tags      map[string]string // additional tags for scope
	}

	// ServiceIdx is an index that uniquely identifies the service
	ServiceIdx int

	// ScopeIdx is an index that uniquely identifies an operation, which is required to form a new metrics scope
	ScopeIdx int

	// MetricIdx is an index that uniquely identifies the metric definition
	MetricIdx int
)

func (s scopeDefinition) GetOperationString() string {
	return s.operation
}

// MetricTypes which are supported
const (
	Counter MetricType = iota
	Timer
	Gauge
	Histogram
)

// Service names for all services that emit metrics.
const (
	Common ServiceIdx = iota
	ShardDistributor
)

// This package should hold all the metrics and tags for cadence
// Note that to better support Prometheus, metric name and tag name
// should match the regex [a-zA-Z_][a-zA-Z0-9_]*, tag value can be any Unicode characters.
// See more https://prometheus.io/docs/concepts/data_model/#metric-names-and-labels

// Common tags for all services
const (
	OperationTagName      = "operation"
	CadenceServiceTagName = "cadence_service"
)

// Common service base metrics
const (
	RestartCount         = "restarts"
	NumGoRoutinesGauge   = "num_goroutines"
	GoMaxProcsGauge      = "gomaxprocs"
	MemoryAllocatedGauge = "memory_allocated"
	MemoryHeapGauge      = "memory_heap"
	MemoryHeapIdleGauge  = "memory_heapidle"
	MemoryHeapInuseGauge = "memory_heapinuse"
	MemoryStackGauge     = "memory_stack"
	NumGCCounter         = "memory_num_gc"
	GcPauseMsTimer       = "memory_gc_pause_ms"
)

// ServiceMetrics are types for common service base metrics
var ServiceMetrics = map[MetricName]MetricType{
	RestartCount: Counter,
}

// GoRuntimeMetrics represent the runtime stats from go runtime
var GoRuntimeMetrics = map[MetricName]MetricType{
	NumGoRoutinesGauge:   Gauge,
	GoMaxProcsGauge:      Gauge,
	MemoryAllocatedGauge: Gauge,
	MemoryHeapGauge:      Gauge,
	MemoryHeapIdleGauge:  Gauge,
	MemoryHeapInuseGauge: Gauge,
	MemoryStackGauge:     Gauge,
	NumGCCounter:         Counter,
	GcPauseMsTimer:       Timer,
}

// Scopes enum
const (
	// -- Common Operation scopes --

	// ShardDistributorClientGetShardOwnerScope tracks GetShardOwner calls made by service to shard distributor
	ShardDistributorClientGetShardOwnerScope ScopeIdx = iota

	// ShardDistributorClientGetNamespaceStateScope tracks GetNamespaceState calls made by service to shard distributor
	ShardDistributorClientGetNamespaceStateScope

	// ShardDistributorClientGetFullNamespaceStateScope tracks GetFullNamespaceState calls made by service to shard distributor
	ShardDistributorClientGetFullNamespaceStateScope

	// ShardDistributorClientGetExecutorStateScope tracks GetExecutorState calls made by service to shard distributor
	ShardDistributorClientGetExecutorStateScope

	// ShardDistributorClientListNamespacesScope tracks ListNamespaces calls made by service to shard distributor
	ShardDistributorClientListNamespacesScope

	// ShardDistributorClientWatchNamespaceStateScope tracks WatchNamespaceState calls made by service to shard distributor
	ShardDistributorClientWatchNamespaceStateScope

	// ShardDistributorClientInspectShardScope tracks InspectShard calls made by service to shard distributor
	ShardDistributorClientInspectShardScope

	// ShardDistributorClientDrainShardsScope tracks DrainShards calls made by service to shard distributor
	ShardDistributorClientDrainShardsScope

	// ShardDistributorClientUndrainShardsScope tracks UndrainShards calls made by service to shard distributor
	ShardDistributorClientUndrainShardsScope

	// ShardDistributorClientGetDrainedShardsScope tracks GetDrainedShards calls made by service to shard distributor
	ShardDistributorClientGetDrainedShardsScope

	// ShardDistributorClientForceResetNamespaceScope tracks ForceResetNamespace calls made by service to shard distributor
	ShardDistributorClientForceResetNamespaceScope

	// ShardDistributorExecutorClientHeartbeatScope tracks Heartbeat calls made by executor to shard distributor
	ShardDistributorExecutorClientHeartbeatScope

	NumCommonScopes
)

// -- Operation scopes for ShardDistributor service --
const (
	// ShardDistributorGetShardOwnerScope tracks GetShardOwner API calls received by service
	ShardDistributorGetShardOwnerScope = iota + NumCommonScopes
	ShardDistributorGetNamespaceStateScope
	ShardDistributorGetFullNamespaceStateScope
	ShardDistributorGetExecutorStateScope
	ShardDistributorListNamespacesScope
	ShardDistributorWatchNamespaceStateScope
	ShardDistributorHeartbeatScope
	ShardDistributorAssignLoopScope
	ShardDistributorDrainShardsScope
	ShardDistributorUndrainShardsScope
	ShardDistributorGetDrainedShardsScope

	ShardDistributorStoreAssignShardsScope
	ShardDistributorStoreDeleteExecutorsScope
	ShardDistributorStoreDeleteShardStatsScope
	ShardDistributorStoreGetExecutorStateScope
	ShardDistributorStoreGetStateScope
	ShardDistributorStoreGetAssignmentStateScope
	ShardDistributorStoreRecordHeartbeatScope
	ShardDistributorStoreRecordShardStatisticsScope
	ShardDistributorStoreRecordShardStatisticsBatchScope
	ShardDistributorStoreSubscribeToExecutorStatusChangesScope
	ShardDistributorStoreDeleteAssignedStatesScope
	ShardDistributorStoreResetNamespaceScope
	ShardDistributorStoreDrainShardsScope
	ShardDistributorStoreUndrainShardsScope
	ShardDistributorStoreGetDrainedShardsScope
	ShardDistributorStoreDrainHostsScope
	ShardDistributorStoreUndrainHostsScope
	ShardDistributorStoreGetDrainedHostsScope

	// ShardDistributorWatchScope tracks etcd watch stream processing
	ShardDistributorWatchScope

	// ShardDistributorLeaderScope tracks leader election state
	ShardDistributorLeaderScope

	// ShardDistributorInspectShardScope tracks InspectShard API calls received by service
	ShardDistributorInspectShardScope

	// ShardDistributorForceResetNamespaceScope tracks ForceResetNamespace API calls received by service
	ShardDistributorForceResetNamespaceScope

	// ShardDistributorEphemeralAssignmentScope tracks on-demand ephemeral assignment batches.
	ShardDistributorEphemeralAssignmentScope

	NumShardDistributorScopes
)

// ScopeDefs record the scopes for all services
var ScopeDefs = map[ServiceIdx]map[ScopeIdx]scopeDefinition{
	Common: {
		ShardDistributorClientGetShardOwnerScope:         {operation: "ShardDistributorClientGetShardOwner"},
		ShardDistributorClientGetNamespaceStateScope:     {operation: "ShardDistributorClientGetNamespaceState"},
		ShardDistributorClientGetFullNamespaceStateScope: {operation: "ShardDistributorClientGetFullNamespaceState"},
		ShardDistributorClientGetExecutorStateScope:      {operation: "ShardDistributorClientGetExecutorState"},
		ShardDistributorClientListNamespacesScope:        {operation: "ShardDistributorClientListNamespaces"},
		ShardDistributorClientWatchNamespaceStateScope:   {operation: "ShardDistributorClientWatchNamespaceState"},
		ShardDistributorClientInspectShardScope:          {operation: "ShardDistributorClientInspectShard"},
		ShardDistributorClientDrainShardsScope:           {operation: "ShardDistributorClientDrainShards"},
		ShardDistributorClientUndrainShardsScope:         {operation: "ShardDistributorClientUndrainShards"},
		ShardDistributorClientGetDrainedShardsScope:      {operation: "ShardDistributorClientGetDrainedShards"},
		ShardDistributorClientForceResetNamespaceScope:   {operation: "ShardDistributorClientForceResetNamespace"},
		ShardDistributorExecutorClientHeartbeatScope:     {operation: "ShardDistributorExecutorHeartbeat"},
	},
	ShardDistributor: {
		ShardDistributorGetShardOwnerScope:                         {operation: "GetShardOwner"},
		ShardDistributorGetNamespaceStateScope:                     {operation: "GetNamespaceState"},
		ShardDistributorGetFullNamespaceStateScope:                 {operation: "GetFullNamespaceState"},
		ShardDistributorGetExecutorStateScope:                      {operation: "GetExecutorState"},
		ShardDistributorListNamespacesScope:                        {operation: "ListNamespaces"},
		ShardDistributorWatchNamespaceStateScope:                   {operation: "WatchNamespaceState"},
		ShardDistributorHeartbeatScope:                             {operation: "ExecutorHeartbeat"},
		ShardDistributorAssignLoopScope:                            {operation: "ShardAssignLoop"},
		ShardDistributorDrainShardsScope:                           {operation: "DrainShards"},
		ShardDistributorUndrainShardsScope:                         {operation: "UndrainShards"},
		ShardDistributorGetDrainedShardsScope:                      {operation: "GetDrainedShards"},
		ShardDistributorStoreAssignShardsScope:                     {operation: "StoreAssignShards"},
		ShardDistributorStoreDeleteExecutorsScope:                  {operation: "StoreDeleteExecutors"},
		ShardDistributorStoreDeleteShardStatsScope:                 {operation: "StoreDeleteShardStats"},
		ShardDistributorStoreGetExecutorStateScope:                 {operation: "StoreGetExecutorState"},
		ShardDistributorStoreGetStateScope:                         {operation: "StoreGetState"},
		ShardDistributorStoreGetAssignmentStateScope:               {operation: "StoreGetAssignmentState"},
		ShardDistributorStoreRecordHeartbeatScope:                  {operation: "StoreRecordHeartbeat"},
		ShardDistributorStoreRecordShardStatisticsScope:            {operation: "StoreRecordShardStatistics"},
		ShardDistributorStoreRecordShardStatisticsBatchScope:       {operation: "StoreRecordShardStatisticsBatch"},
		ShardDistributorStoreSubscribeToExecutorStatusChangesScope: {operation: "StoreSubscribeToExecutorStatusChanges"},
		ShardDistributorStoreDeleteAssignedStatesScope:             {operation: "StoreDeleteAssignedStates"},
		ShardDistributorStoreResetNamespaceScope:                   {operation: "StoreResetNamespace"},
		ShardDistributorStoreDrainShardsScope:                      {operation: "StoreDrainShards"},
		ShardDistributorStoreUndrainShardsScope:                    {operation: "StoreUndrainShards"},
		ShardDistributorStoreGetDrainedShardsScope:                 {operation: "StoreGetDrainedShards"},
		ShardDistributorStoreDrainHostsScope:                       {operation: "StoreDrainHosts"},
		ShardDistributorStoreUndrainHostsScope:                     {operation: "StoreUndrainHosts"},
		ShardDistributorStoreGetDrainedHostsScope:                  {operation: "StoreGetDrainedHosts"},
		ShardDistributorWatchScope:                                 {operation: "Watch"},
		ShardDistributorLeaderScope:                                {operation: "Leader"},
		ShardDistributorInspectShardScope:                          {operation: "InspectShard"},
		ShardDistributorForceResetNamespaceScope:                   {operation: "ForceResetNamespace"},
		ShardDistributorEphemeralAssignmentScope:                   {operation: "EphemeralAssignment"},
	},
}

// Common Metrics enum
const (
	CadenceClientRequests MetricIdx = iota
	CadenceClientFailures
	CadenceClientLatency

	NumCommonMetrics
)

// ShardDistributor metrics enum
const (
	ShardDistributorRequests = iota + NumCommonMetrics
	ShardDistributorFailures
	ShardDistributorLatency
	ShardDistributorErrContextTimeoutCounter
	ShardDistributorErrNamespaceNotFound
	ShardDistributorErrShardNotFound

	ShardDistributorAssignLoopNumRebalancedShards
	ShardDistributorAssignLoopShardRebalanceLatency
	ShardDistributorAssignLoopAttempts
	ShardDistributorAssignLoopSuccess
	ShardDistributorAssignLoopFail

	ShardDistributorActiveShards
	ShardDistributorTotalExecutors
	ShardDistributorOldestExecutorHeartbeatLag
	ShardDistributorMaxExecutorsPerShard

	ShardDistributorStoreExecutorNotFound
	ShardDistributorStoreShardStatisticsSkipped
	ShardDistributorStoreFailuresPerNamespace
	ShardDistributorStoreRequestsPerNamespace
	ShardDistributorStoreLatencyHistogramPerNamespace
	ShardDistributorStoreGetStateETCDRoundTripLatency
	ShardDistributorStoreGetAssignmentStateETCDRoundTripLatency

	ShardDistributorEphemeralAssignmentBatchSize
	ShardDistributorEphemeralAssignmentWriteAttempts

	// ShardDistributorShardAssignmentDistributionLatency measures the time taken between assignment of a shard
	// and the time it is fully distributed to executors
	ShardDistributorShardAssignmentDistributionLatency

	// ShardDistributorShardHandoverLatency measures the time taken to hand over a shard from one executor to another
	ShardDistributorShardHandoverLatency

	// ShardDistributorWatchProcessingLatency measures how long it takes to process a single WatchResponse
	ShardDistributorWatchProcessingLatency
	// ShardDistributorWatchEventsReceived counts the total number of watch events received
	ShardDistributorWatchEventsReceived

	// ShardDistributorAssignLoopLoadBasedMoves counts the number of shards moved due to load rebalancing
	ShardDistributorAssignLoopLoadBasedMoves
	// ShardDistributorAssignLoopDeletedShards counts the number of shards removed (DONE status) in a rebalance cycle
	ShardDistributorAssignLoopDeletedShards
	// ShardDistributorAssignLoopMovedShardLoad counts the reported load of shards moved due to load rebalancing
	ShardDistributorAssignLoopMovedShardLoad
	// ShardDistributorAssignLoopDroppedDrainedShards counts drained shards taken away from an executor in a rebalance cycle
	ShardDistributorAssignLoopDroppedDrainedShards
	// ShardDistributorDrainedShards tracks how many shards are currently drained in the namespace
	ShardDistributorDrainedShards

	// ShardDistributorAssignmentLoadMaxOverMean measures max/mean across executor reported loads
	ShardDistributorAssignmentLoadMaxOverMean
	// ShardDistributorAssignmentLoadCV measures coefficient of variation across executor reported loads
	ShardDistributorAssignmentLoadCV
	// ShardDistributorAssignmentSmoothedLoadMaxOverMean measures max/mean across executor smoothed loads
	ShardDistributorAssignmentSmoothedLoadMaxOverMean
	// ShardDistributorAssignmentSmoothedLoadCV measures coefficient of variation across executor smoothed loads
	ShardDistributorAssignmentSmoothedLoadCV
	// ShardDistributorAssignmentSmoothedLoadMissingRatio measures the fraction of assigned shards with no smoothed load
	ShardDistributorAssignmentSmoothedLoadMissingRatio
	// ShardDistributorIsLeader reports whether this instance is currently the leader (1) or not (0) for a namespace
	ShardDistributorIsLeader

	// ShardDistributorAssignLoopNoActiveExecutors counts rebalance cycles that found no active executors
	ShardDistributorAssignLoopNoActiveExecutors

	// ShardDistributorErrContextCanceledCounter counts requests terminated by a canceled context or a
	// closed stream. These are expected terminations (client disconnect, server shutdown), not failures.
	ShardDistributorErrContextCanceledCounter

	NumShardDistributorMetrics
)

// MetricDefs record the metrics for all services
var MetricDefs = map[ServiceIdx]map[MetricIdx]metricDefinition{
	Common: {
		CadenceClientRequests: {metricName: "cadence_client_requests", metricType: Counter},
		CadenceClientFailures: {metricName: "cadence_client_errors", metricType: Counter},
		CadenceClientLatency:  {metricName: "cadence_client_latency", metricType: Timer},
	},
	ShardDistributor: {
		ShardDistributorRequests:                        {metricName: "shard_distributor_requests", metricType: Counter},
		ShardDistributorErrContextTimeoutCounter:        {metricName: "shard_distributor_err_context_timeout", metricType: Counter},
		ShardDistributorFailures:                        {metricName: "shard_distributor_failures", metricType: Counter},
		ShardDistributorLatency:                         {metricName: "shard_distributor_latency", metricType: Timer},
		ShardDistributorErrNamespaceNotFound:            {metricName: "shard_distributor_err_namespace_not_found", metricType: Counter},
		ShardDistributorErrShardNotFound:                {metricName: "shard_distributor_err_shard_not_found", metricType: Counter},
		ShardDistributorAssignLoopShardRebalanceLatency: {metricName: "shard_distrubutor_shard_assign_latency", metricType: Histogram},
		ShardDistributorAssignLoopNumRebalancedShards:   {metricName: "shard_distributor_shard_assign_reassigned_shards", metricType: Gauge},
		ShardDistributorAssignLoopAttempts:              {metricName: "shard_distrubutor_shard_assign_attempt", metricType: Counter},
		ShardDistributorAssignLoopSuccess:               {metricName: "shard_distrubutor_shard_assign_success", metricType: Counter},
		ShardDistributorAssignLoopFail:                  {metricName: "shard_distrubutor_shard_assign_fail", metricType: Counter},

		ShardDistributorActiveShards:               {metricName: "shard_distributor_active_shards", metricType: Gauge},
		ShardDistributorTotalExecutors:             {metricName: "shard_distributor_total_executors", metricType: Gauge},
		ShardDistributorOldestExecutorHeartbeatLag: {metricName: "shard_distributor_oldest_executor_heartbeat_lag", metricType: Gauge},
		ShardDistributorMaxExecutorsPerShard:       {metricName: "shard_distributor_max_executors_per_shard", metricType: Gauge},

		ShardDistributorStoreExecutorNotFound:                       {metricName: "shard_distributor_store_executor_not_found", metricType: Counter},
		ShardDistributorStoreShardStatisticsSkipped:                 {metricName: "shard_distributor_store_shard_statistics_skipped", metricType: Counter},
		ShardDistributorStoreFailuresPerNamespace:                   {metricName: "shard_distributor_store_failures_per_namespace", metricType: Counter},
		ShardDistributorStoreRequestsPerNamespace:                   {metricName: "shard_distributor_store_requests_per_namespace", metricType: Counter},
		ShardDistributorStoreLatencyHistogramPerNamespace:           {metricName: "shard_distributor_store_latency_histogram_per_namespace", metricType: Histogram, buckets: ShardDistributorExecutorStoreLatencyBuckets},
		ShardDistributorStoreGetStateETCDRoundTripLatency:           {metricName: "shard_distributor_store_get_state_etcd_round_trip_latency", metricType: Histogram, buckets: ShardDistributorExecutorStoreLatencyBuckets},
		ShardDistributorStoreGetAssignmentStateETCDRoundTripLatency: {metricName: "shard_distributor_store_get_assignment_state_etcd_round_trip_latency", metricType: Histogram, buckets: ShardDistributorExecutorStoreLatencyBuckets},

		ShardDistributorEphemeralAssignmentBatchSize:     {metricName: "shard_distributor_ephemeral_assignment_batch_size", metricType: Histogram, buckets: ShardDistributorEphemeralAssignmentBatchSizeBuckets},
		ShardDistributorEphemeralAssignmentWriteAttempts: {metricName: "shard_distributor_ephemeral_assignment_write_attempts", metricType: Counter},

		ShardDistributorShardAssignmentDistributionLatency: {metricName: "shard_distributor_shard_assignment_distribution_latency", metricType: Histogram, buckets: ShardDistributorShardAssignmentLatencyBuckets},
		ShardDistributorShardHandoverLatency:               {metricName: "shard_distributor_shard_handover_latency", metricType: Histogram, buckets: ShardDistributorShardAssignmentLatencyBuckets},

		ShardDistributorWatchProcessingLatency: {metricName: "shard_distributor_watch_processing_latency", metricType: Histogram, buckets: Default1ms100s.buckets()},
		ShardDistributorWatchEventsReceived:    {metricName: "shard_distributor_watch_events_received", metricType: Counter},

		ShardDistributorAssignLoopLoadBasedMoves: {metricName: "shard_distributor_shard_assign_load_based_moves", metricType: Counter},
		ShardDistributorAssignLoopDeletedShards:  {metricName: "shard_distributor_shard_assign_deleted_shards", metricType: Gauge},
		ShardDistributorAssignLoopMovedShardLoad: {metricName: "shard_distributor_shard_assign_moved_load", metricType: Counter},

		ShardDistributorAssignLoopDroppedDrainedShards: {metricName: "shard_distributor_shard_assign_dropped_drained_shards", metricType: Counter},
		ShardDistributorDrainedShards:                  {metricName: "shard_distributor_drained_shards", metricType: Gauge},

		ShardDistributorAssignmentLoadMaxOverMean:         {metricName: "shard_distributor_assignment_load_max_over_mean", metricType: Gauge},
		ShardDistributorAssignmentLoadCV:                  {metricName: "shard_distributor_assignment_load_cv", metricType: Gauge},
		ShardDistributorAssignmentSmoothedLoadMaxOverMean: {metricName: "shard_distributor_assignment_smoothed_load_max_over_mean", metricType: Gauge},
		ShardDistributorAssignmentSmoothedLoadCV:          {metricName: "shard_distributor_assignment_smoothed_load_cv", metricType: Gauge},
		ShardDistributorAssignmentSmoothedLoadMissingRatio: {
			metricName: "shard_distributor_assignment_smoothed_load_missing_ratio",
			metricType: Gauge,
		},
		ShardDistributorIsLeader:                    {metricName: "shard_distributor_is_leader", metricType: Gauge},
		ShardDistributorAssignLoopNoActiveExecutors: {metricName: "shard_distributor_shard_assign_no_active_executors", metricType: Counter},
		ShardDistributorErrContextCanceledCounter:   {metricName: "shard_distributor_err_context_canceled", metricType: Counter},
	},
}

var (
	ShardDistributorExecutorStoreLatencyBuckets = tally.DurationBuckets([]time.Duration{
		0,
		5 * time.Millisecond,
		10 * time.Millisecond,
		25 * time.Millisecond,
		50 * time.Millisecond,
		75 * time.Millisecond,
		100 * time.Millisecond,
		120 * time.Millisecond,
		150 * time.Millisecond,
		170 * time.Millisecond,
		200 * time.Millisecond,
		250 * time.Millisecond,
		300 * time.Millisecond,
		400 * time.Millisecond,
		500 * time.Millisecond,
		600 * time.Millisecond,
		700 * time.Millisecond,
		800 * time.Millisecond,
		900 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
		3 * time.Second,
		4 * time.Second,
		5 * time.Second,
		6 * time.Second,
		7 * time.Second,
		8 * time.Second,
		9 * time.Second,
		10 * time.Second,
		12 * time.Second,
		15 * time.Second,
		20 * time.Second,
		25 * time.Second,
		30 * time.Second,
		35 * time.Second,
		40 * time.Second,
		50 * time.Second,
		60 * time.Second,
	})

	ShardDistributorEphemeralAssignmentBatchSizeBuckets = tally.MustMakeExponentialValueBuckets(1, 2, 11) // 1..1024

	ShardDistributorShardAssignmentLatencyBuckets = tally.DurationBuckets([]time.Duration{
		// ShardDistributorShardHandoverLatency for GracefulHandoverType should be within 0s and 1s

		0,
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
		400 * time.Millisecond,
		500 * time.Millisecond,
		600 * time.Millisecond,
		700 * time.Millisecond,
		800 * time.Millisecond,
		900 * time.Millisecond,

		// ShardDistributorShardHandoverLatency for EmergencyHandoverType should be within 0s and 10s
		1 * time.Second,
		2 * time.Second,
		3 * time.Second,
		4 * time.Second,
		5 * time.Second,
		6 * time.Second,
		7 * time.Second,
		8 * time.Second,
		9 * time.Second,
		10 * time.Second,

		12 * time.Second,
		15 * time.Second,
		20 * time.Second,
		30 * time.Second,
		45 * time.Second,

		1 * time.Minute,
		2 * time.Minute,
		5 * time.Minute,
		10 * time.Minute,
	})
)

// Empty returns true if the metricName is an empty string
func (mn MetricName) Empty() bool {
	return mn == ""
}

func (mn MetricName) String() string {
	return string(mn)
}
