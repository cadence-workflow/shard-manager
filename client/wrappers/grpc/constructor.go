// The MIT License (MIT)

// Copyright (c) 2017-2020 Uber Technologies Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package grpc

import (
	adminv1 "github.com/uber/cadence-idl/go/proto/admin/v1"
	apiv1 "github.com/uber/cadence-idl/go/proto/api/v1"

	historyv1 "github.com/uber/cadence/.gen/proto/history/v1"
	matchingv1 "github.com/uber/cadence/.gen/proto/matching/v1"
	sharddistributorv1 "github.com/uber/cadence/.gen/proto/sharddistributor/v1"
	"github.com/uber/cadence/client/sharddistributor"
	"github.com/uber/cadence/client/sharddistributorexecutor"
)

type (
	adminClient struct {
		c adminv1.AdminAPIYARPCClient
	}

	frontendGRPCClientWrapper struct {
		apiv1.DomainAPIYARPCClient
		apiv1.WorkflowAPIYARPCClient
		apiv1.WorkerAPIYARPCClient
		apiv1.VisibilityAPIYARPCClient
	}
	frontendClient struct {
		c *frontendGRPCClientWrapper
	}
	historyClient struct {
		c historyv1.HistoryAPIYARPCClient
	}
	matchingClient struct {
		c matchingv1.MatchingAPIYARPCClient
	}
	sharddistributorClient struct {
		c sharddistributorv1.ShardDistributorAPIYARPCClient
	}
	sharddistributorexecutorClient struct {
		c sharddistributorv1.ShardDistributorExecutorAPIYARPCClient
	}
)

func NewShardDistributorClient(c sharddistributorv1.ShardDistributorAPIYARPCClient) sharddistributor.Client {
	return sharddistributorClient{c}
}

func NewShardDistributorExecutorClient(c sharddistributorv1.ShardDistributorExecutorAPIYARPCClient) sharddistributorexecutor.Client {
	return sharddistributorexecutorClient{c}
}
