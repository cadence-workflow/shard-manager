package metricsconstants

const (
	// Operation tag names for ShardDistributorSpectator metrics
	ShardDistributorSpectatorOperationTagName                    = "ShardDistributorSpectator"
	ShardDistributorSpectatorGetShardOwnerOperationTagName       = "ShardDistributorSpectatorGetShardOwner"
	ShardDistributorSpectatorInspectShardOperationTagName        = "ShardDistributorSpectatorInspectShard"
	ShardDistributorSpectatorGetNamespaceStateOperationTagName   = "ShardDistributorSpectatorGetNamespaceState"
	ShardDistributorSpectatorGetExecutorStateOperationTagName    = "ShardDistributorSpectatorGetExecutorState"
	ShardDistributorSpectatorListNamespacesOperationTagName      = "ShardDistributorSpectatorListNamespaces"
	ShardDistributorSpectatorWatchNamespaceStateOperationTagName = "ShardDistributorSpectatorWatchNamespaceState"
	ShardDistributorSpectatorDrainShardsOperationTagName         = "ShardDistributorSpectatorDrainShards"
	ShardDistributorSpectatorUndrainShardsOperationTagName       = "ShardDistributorSpectatorUndrainShards"
	ShardDistributorSpectatorGetDrainedShardsOperationTagName    = "ShardDistributorSpectatorGetDrainedShards"
	ShardDistributorSpectatorDrainHostsOperationTagName          = "ShardDistributorSpectatorDrainHosts"
	ShardDistributorSpectatorUndrainHostsOperationTagName        = "ShardDistributorSpectatorUndrainHosts"
	ShardDistributorSpectatorGetDrainedHostsOperationTagName     = "ShardDistributorSpectatorGetDrainedHosts"
	ShardDistributorSpectatorForceResetNamespaceOperationTagName = "ShardDistributorSpectatorForceResetNamespace"

	// Counter metrics
	ShardDistributorSpectatorClientRequests   = "shard_distributor_spectator_client_requests"
	ShardDistributorSpectatorClientFailures   = "shard_distributor_spectator_client_failures"
	ShardDistributorSpectatorStreamReconnects = "shard_distributor_spectator_stream_reconnects"

	// Timer metrics
	ShardDistributorSpectatorClientLatency = "shard_distributor_spectator_client_latency"

	// Tag names
	StreamReconnectReasonTagName = "reason"

	// Tag values for StreamReconnectReasonTagName: timeout is the periodic forced reconnect,
	// error is an unexpected stream failure (network, server issue, etc.).
	StreamReconnectReasonTimeout = "timeout"
	StreamReconnectReasonError   = "error"
)
