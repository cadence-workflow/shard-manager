package tag

var (
	WorkflowActionWorkflowStarted = workflowAction("add-workflow-started-event")
)

var (
	LifeCycleStarting    = lifecycle("Starting")
	LifeCycleStarted     = lifecycle("Started")
	LifeCycleStartFailed = lifecycle("StartFailed")
	LifeCycleStopping    = lifecycle("Stopping")
	LifeCycleStopped     = lifecycle("Stopped")
)

var (
	ComponentNamespaceManager = component("shard-namespace-manager")
	ComponentLeaderElection   = component("shard-leader-election")
	ComponentLeaderProcessor  = component("shard-leader-processor")
)

var (
	ShardDistributorClientOperationGetShardOwner            = clientOperation("shard-distributor-get-shard-owner")
	ShardDistributorClientOperationInspectShard             = clientOperation("shard-distributor-inspect-shard")
	ShardDistributorClientOperationGetNamespaceState        = clientOperation("shard-distributor-get-namespace-state")
	ShardDistributorClientOperationGetFullNamespaceState    = clientOperation("shard-distributor-get-full-namespace-state")
	ShardDistributorClientOperationGetExecutorState         = clientOperation("shard-distributor-get-executor-state")
	ShardDistributorClientOperationListNamespaces           = clientOperation("shard-distributor-list-namespaces")
	ShardDistributorClientOperationWatchNamespaceState      = clientOperation("shard-distributor-watch-namespace-state")
	ShardDistributorClientOperationGetNamespaceStateIfNewer = clientOperation("shard-distributor-get-namespace-state-if-newer")
	ShardDistributorClientOperationDrainShards              = clientOperation("shard-distributor-drain-shards")
	ShardDistributorClientOperationUndrainShards            = clientOperation("shard-distributor-undrain-shards")
	ShardDistributorClientOperationGetDrainedShards         = clientOperation("shard-distributor-get-drained-shards")
	ShardDistributorClientOperationDrainHosts               = clientOperation("shard-distributor-drain-hosts")
	ShardDistributorClientOperationUndrainHosts             = clientOperation("shard-distributor-undrain-hosts")
	ShardDistributorClientOperationGetDrainedHosts          = clientOperation("shard-distributor-get-drained-hosts")
	ShardDistributorClientOperationForceResetNamespace      = clientOperation("shard-distributor-force-reset-namespace")
	ShardDistributorExecutorClientOperationHeartbeat        = clientOperation("shard-distributor-executor-heartbeat")
)
