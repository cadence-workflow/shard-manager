// Copyright (c) 2019 Uber Technologies, Inc.
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

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination resource_mock.go -self_package github.com/cadence-workflow/shard-manager/common/resource

package resource

import (
	"github.com/uber-go/tally"
	"go.uber.org/cadence/.gen/go/cadence/workflowserviceclient"
	"go.uber.org/yarpc"

	"github.com/cadence-workflow/shard-manager/client"
	"github.com/cadence-workflow/shard-manager/client/admin"
	"github.com/cadence-workflow/shard-manager/client/frontend"
	"github.com/cadence-workflow/shard-manager/client/history"
	"github.com/cadence-workflow/shard-manager/client/matching"
	"github.com/cadence-workflow/shard-manager/common"
	"github.com/cadence-workflow/shard-manager/common/activecluster"
	"github.com/cadence-workflow/shard-manager/common/archiver"
	"github.com/cadence-workflow/shard-manager/common/archiver/provider"
	"github.com/cadence-workflow/shard-manager/common/asyncworkflow/queue"
	"github.com/cadence-workflow/shard-manager/common/blobstore"
	"github.com/cadence-workflow/shard-manager/common/cache"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/cluster"
	"github.com/cadence-workflow/shard-manager/common/domain"
	"github.com/cadence-workflow/shard-manager/common/dynamicconfig/configstore"
	"github.com/cadence-workflow/shard-manager/common/isolationgroup"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/membership"
	"github.com/cadence-workflow/shard-manager/common/messaging"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/persistence"
	persistenceClient "github.com/cadence-workflow/shard-manager/common/persistence/client"
	qrpc "github.com/cadence-workflow/shard-manager/common/quotas/global/rpc"
	"github.com/cadence-workflow/shard-manager/common/service"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient"
)

type ResourceFactory interface {
	NewResource(params *Params,
		serviceName string,
		serviceConfig *service.Config,
	) (resource Resource, err error)
}

// Resource is the interface which expose common resources
type Resource interface {
	common.Daemon

	// static infos

	GetServiceName() string
	GetHostInfo() membership.HostInfo
	GetArchivalMetadata() archiver.ArchivalMetadata
	GetClusterMetadata() cluster.Metadata

	// other common resources

	GetDomainCache() cache.DomainCache
	GetDomainMetricsScopeCache() cache.DomainMetricsScopeCache
	GetActiveClusterManager() activecluster.Manager
	GetTimeSource() clock.TimeSource
	GetPayloadSerializer() persistence.PayloadSerializer
	GetMetricsClient() metrics.Client
	GetArchiverProvider() provider.ArchiverProvider
	GetMessagingClient() messaging.Client
	GetBlobstoreClient() blobstore.Client
	GetDomainReplicationQueue() domain.ReplicationQueue

	// membership infos
	GetMembershipResolver() membership.Resolver

	// internal services clients

	GetSDKClient() workflowserviceclient.Interface
	GetFrontendRawClient() frontend.Client
	GetFrontendClient() frontend.Client
	GetMatchingRawClient() matching.Client
	GetMatchingClient() matching.Client
	GetHistoryRawClient() history.Client
	GetHistoryClient() history.Client
	GetRatelimiterAggregatorsClient() qrpc.Client
	GetRemoteAdminClient(cluster string) (admin.Client, error)
	GetRemoteFrontendClient(cluster string) (frontend.Client, error)
	GetClientBean() client.Bean
	GetShardDistributorExecutorClient() executorclient.Client

	// persistence clients
	GetDomainManager() persistence.DomainManager
	GetDomainAuditManager() persistence.DomainAuditManager
	GetTaskManager() persistence.TaskManager
	GetVisibilityManager() persistence.VisibilityManager
	GetShardManager() persistence.ShardManager
	GetHistoryManager() persistence.HistoryManager
	GetExecutionManager(int) (persistence.ExecutionManager, error)
	GetPersistenceBean() persistenceClient.Bean

	// GetHostName get host name
	GetHostName() string

	// loggers
	GetLogger() log.Logger
	GetThrottledLogger() log.Logger

	// for registering handlers
	GetDispatcher() *yarpc.Dispatcher

	// GetIsolationGroupState returns the isolationGroupState
	GetIsolationGroupState() isolationgroup.State
	GetIsolationGroupStore() configstore.Client

	GetAsyncWorkflowQueueProvider() queue.Provider

	// GetMetricsScope returns the tally scope for metrics reporting
	GetMetricsScope() tally.Scope
}
