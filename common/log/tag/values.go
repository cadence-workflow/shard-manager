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

package tag

var (
	WorkflowActionWorkflowStarted = workflowAction("add-workflow-started-event")
)

var (
	ComponentNamespaceManager = component("shard-namespace-manager")
	ComponentLeaderElection   = component("shard-leader-election")
	ComponentLeaderProcessor  = component("shard-leader-processor")
)

var (
	ShardDistributorClientOperationGetShardOwner         = clientOperation("shard-distributor-get-shard-owner")
	ShardDistributorClientOperationInspectShard          = clientOperation("shard-distributor-inspect-shard")
	ShardDistributorClientOperationGetNamespaceState     = clientOperation("shard-distributor-get-namespace-state")
	ShardDistributorClientOperationGetFullNamespaceState = clientOperation("shard-distributor-get-full-namespace-state")
	ShardDistributorClientOperationGetExecutorState      = clientOperation("shard-distributor-get-executor-state")
	ShardDistributorClientOperationListNamespaces        = clientOperation("shard-distributor-list-namespaces")
	ShardDistributorClientOperationWatchNamespaceState   = clientOperation("shard-distributor-watch-namespace-state")
	ShardDistributorClientOperationDrainShards           = clientOperation("shard-distributor-drain-shards")
	ShardDistributorClientOperationUndrainShards         = clientOperation("shard-distributor-undrain-shards")
	ShardDistributorClientOperationGetDrainedShards      = clientOperation("shard-distributor-get-drained-shards")
	ShardDistributorClientOperationForceResetNamespace   = clientOperation("shard-distributor-force-reset-namespace")
	ShardDistributorExecutorClientOperationHeartbeat     = clientOperation("shard-distributor-executor-heartbeat")
)
