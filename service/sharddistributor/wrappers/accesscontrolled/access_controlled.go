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

// Package accesscontrolled wraps a handler.Handler with per-RPC permission checks
// using authorization.Authorizer. Only RPCs that need a permission check are
// overridden: the read APIs (GetNamespaceState, GetExecutorState, InspectShard and
// GetDrainedShards) require PermissionRead, and the administrative APIs
// (ListNamespaces, ForceResetNamespace, DrainShards and UndrainShards) require
// PermissionAdmin. The remaining methods (Health, lifecycle Start/Stop, the
// executor hot-path GetShardOwner, and WatchNamespaceState) flow through the
// embedded handler.Handler unchecked.
package accesscontrolled

import (
	"context"

	"github.com/cadence-workflow/shard-manager/common/authorization"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/handler"
)

var errUnauthorized = &types.AccessDeniedError{Message: "Request unauthorized."}

type accessControlledHandler struct {
	handler.Handler
	authorizer authorization.Authorizer
}

// NewHandler returns a handler.Handler that calls authorizer.Authorize before
// dispatching each RPC to the wrapped handler. A nil authorizer is treated as
// the no-op authorizer (every request allowed) so callers can pass through
// fx-optional dependencies without nil-checks.
func NewHandler(h handler.Handler, authorizer authorization.Authorizer) handler.Handler {
	if authorizer == nil {
		authorizer = authorization.NewNopAuthorizer()
	}
	return &accessControlledHandler{
		Handler:    h,
		authorizer: authorizer,
	}
}

func (a *accessControlledHandler) InspectShard(ctx context.Context, req *types.GetShardOwnerRequest) (*types.GetShardOwnerResponse, error) {
	if err := a.authorize(ctx, "InspectShard", req.GetNamespace(), authorization.PermissionRead); err != nil {
		return nil, err
	}
	return a.Handler.InspectShard(ctx, req)
}

func (a *accessControlledHandler) GetNamespaceState(ctx context.Context, req *types.GetNamespaceStateRequest) (*types.GetNamespaceStateResponse, error) {
	if err := a.authorize(ctx, "GetNamespaceState", req.GetNamespace(), authorization.PermissionRead); err != nil {
		return nil, err
	}
	return a.Handler.GetNamespaceState(ctx, req)
}

func (a *accessControlledHandler) GetExecutorState(ctx context.Context, req *types.GetExecutorStateRequest) (*types.GetExecutorStateResponse, error) {
	if err := a.authorize(ctx, "GetExecutorState", req.GetNamespace(), authorization.PermissionRead); err != nil {
		return nil, err
	}
	return a.Handler.GetExecutorState(ctx, req)
}

func (a *accessControlledHandler) ListNamespaces(ctx context.Context, req *types.ListNamespacesRequest) (*types.ListNamespacesResponse, error) {
	if err := a.authorize(ctx, "ListNamespaces", "", authorization.PermissionAdmin); err != nil {
		return nil, err
	}
	return a.Handler.ListNamespaces(ctx, req)
}

func (a *accessControlledHandler) ForceResetNamespace(ctx context.Context, req *types.ForceResetNamespaceRequest) (*types.ForceResetNamespaceResponse, error) {
	if err := a.authorize(ctx, "ForceResetNamespace", req.GetNamespace(), authorization.PermissionAdmin); err != nil {
		return nil, err
	}
	return a.Handler.ForceResetNamespace(ctx, req)
}

func (a *accessControlledHandler) DrainShards(ctx context.Context, req *types.DrainShardsRequest) error {
	if err := a.authorize(ctx, "DrainShards", req.GetNamespace(), authorization.PermissionAdmin); err != nil {
		return err
	}
	return a.Handler.DrainShards(ctx, req)
}

func (a *accessControlledHandler) UndrainShards(ctx context.Context, req *types.UndrainShardsRequest) (*types.UndrainShardsResponse, error) {
	if err := a.authorize(ctx, "UndrainShards", req.GetNamespace(), authorization.PermissionAdmin); err != nil {
		return nil, err
	}
	return a.Handler.UndrainShards(ctx, req)
}

func (a *accessControlledHandler) GetDrainedShards(ctx context.Context, req *types.GetDrainedShardsRequest) (*types.GetDrainedShardsResponse, error) {
	if err := a.authorize(ctx, "GetDrainedShards", req.GetNamespace(), authorization.PermissionRead); err != nil {
		return nil, err
	}
	return a.Handler.GetDrainedShards(ctx, req)
}

func (a *accessControlledHandler) authorize(ctx context.Context, apiName, namespace string, permission authorization.Permission) error {
	result, err := a.authorizer.Authorize(ctx, &authorization.Attributes{
		APIName:    apiName,
		Namespace:  namespace,
		Permission: permission,
	})
	if err != nil {
		return err
	}
	if result.Decision != authorization.DecisionAllow {
		return errUnauthorized
	}
	return nil
}
