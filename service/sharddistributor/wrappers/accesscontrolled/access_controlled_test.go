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

package accesscontrolled

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/authorization"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/handler"
)

const testNamespace = "ns1"

var errAuthorizerBoom = errors.New("authorizer boom")

func TestAccessControlledHandler_GetNamespaceState(t *testing.T) {
	tests := []struct {
		name              string
		authorizeResult   authorization.Result
		authorizeErr      error
		expectInnerCalled bool
		expectErr         error
	}{
		{
			name:              "allow -> inner called",
			authorizeResult:   authorization.Result{Decision: authorization.DecisionAllow},
			expectInnerCalled: true,
		},
		{
			name:              "deny -> AccessDeniedError",
			authorizeResult:   authorization.Result{Decision: authorization.DecisionDeny},
			expectInnerCalled: false,
			expectErr:         errUnauthorized,
		},
		{
			name:              "authorizer error -> propagated",
			authorizeErr:      errAuthorizerBoom,
			expectInnerCalled: false,
			expectErr:         errAuthorizerBoom,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			inner := handler.NewMockHandler(ctrl)
			authz := authorization.NewMockAuthorizer(ctrl)

			expectedAttrs := &authorization.Attributes{
				APIName:    "GetNamespaceState",
				Namespace:  testNamespace,
				Permission: authorization.PermissionRead,
			}
			authz.EXPECT().
				Authorize(gomock.Any(), expectedAttrs).
				Return(tc.authorizeResult, tc.authorizeErr).
				Times(1)

			if tc.expectInnerCalled {
				inner.EXPECT().
					GetNamespaceState(gomock.Any(), gomock.Any()).
					Return(&types.GetNamespaceStateResponse{Namespace: testNamespace}, nil).
					Times(1)
			}

			wrapped := NewHandler(inner, authz)
			resp, err := wrapped.GetNamespaceState(context.Background(), &types.GetNamespaceStateRequest{Namespace: testNamespace})

			if tc.expectErr != nil {
				assert.Nil(t, resp)
				assert.ErrorIs(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, testNamespace, resp.Namespace)
		})
	}
}

func TestAccessControlledHandler_GetExecutorState(t *testing.T) {
	tests := []struct {
		name              string
		authorizeResult   authorization.Result
		authorizeErr      error
		expectInnerCalled bool
		expectErr         error
	}{
		{name: "allow -> inner called", authorizeResult: authorization.Result{Decision: authorization.DecisionAllow}, expectInnerCalled: true},
		{name: "deny -> AccessDeniedError", authorizeResult: authorization.Result{Decision: authorization.DecisionDeny}, expectErr: errUnauthorized},
		{name: "authorizer error -> propagated", authorizeErr: errAuthorizerBoom, expectErr: errAuthorizerBoom},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			inner := handler.NewMockHandler(ctrl)
			authz := authorization.NewMockAuthorizer(ctrl)

			authz.EXPECT().
				Authorize(gomock.Any(), &authorization.Attributes{
					APIName:    "GetExecutorState",
					Namespace:  testNamespace,
					Permission: authorization.PermissionRead,
				}).
				Return(tc.authorizeResult, tc.authorizeErr).
				Times(1)

			if tc.expectInnerCalled {
				inner.EXPECT().
					GetExecutorState(gomock.Any(), &types.GetExecutorStateRequest{Namespace: testNamespace, ExecutorID: "executor1"}).
					Return(&types.GetExecutorStateResponse{Namespace: testNamespace}, nil).
					Times(1)
			}

			resp, err := NewHandler(inner, authz).GetExecutorState(
				context.Background(),
				&types.GetExecutorStateRequest{Namespace: testNamespace, ExecutorID: "executor1"},
			)

			if tc.expectErr != nil {
				assert.Nil(t, resp)
				assert.ErrorIs(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, testNamespace, resp.Namespace)
		})
	}
}

func TestAccessControlledHandler_NopDefaults(t *testing.T) {
	ctrl := gomock.NewController(t)
	inner := handler.NewMockHandler(ctrl)
	inner.EXPECT().
		GetNamespaceState(gomock.Any(), gomock.Any()).
		Return(&types.GetNamespaceStateResponse{Namespace: testNamespace}, nil).
		Times(1)

	wrapped := NewHandler(inner, nil) // nil -> nop authorizer

	resp, err := wrapped.GetNamespaceState(context.Background(), &types.GetNamespaceStateRequest{Namespace: testNamespace})
	require.NoError(t, err)
	assert.Equal(t, testNamespace, resp.Namespace)
}

// TestAccessControlledHandler_BypassesAuthForUncheckedMethods asserts that the
// methods we intentionally do NOT override (Health, Start, Stop, GetShardOwner,
// WatchNamespaceState) flow through the embedded handler.Handler without
// invoking the authorizer. The mock authorizer fails the test if Authorize is
// ever called.
func TestAccessControlledHandler_BypassesAuthForUncheckedMethods(t *testing.T) {
	ctrl := gomock.NewController(t)
	inner := handler.NewMockHandler(ctrl)
	authz := authorization.NewMockAuthorizer(ctrl) // no EXPECT -> any call fails the test

	inner.EXPECT().Health(gomock.Any()).Return(&types.HealthStatus{Ok: true}, nil).Times(1)
	inner.EXPECT().Start().Times(1)
	inner.EXPECT().Stop().Times(1)
	inner.EXPECT().
		GetShardOwner(gomock.Any(), gomock.Any()).
		Return(&types.GetShardOwnerResponse{Owner: "host-1"}, nil).
		Times(1)
	inner.EXPECT().
		WatchNamespaceState(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	wrapped := NewHandler(inner, authz)

	status, err := wrapped.Health(context.Background())
	require.NoError(t, err)
	assert.True(t, status.Ok)

	wrapped.Start()
	wrapped.Stop()

	owner, err := wrapped.GetShardOwner(context.Background(), &types.GetShardOwnerRequest{Namespace: testNamespace})
	require.NoError(t, err)
	assert.Equal(t, "host-1", owner.Owner)

	require.NoError(t, wrapped.WatchNamespaceState(&types.WatchNamespaceStateRequest{Namespace: testNamespace}, nil))
}

func TestAccessControlledHandler_ForceResetNamespace(t *testing.T) {
	tests := []struct {
		name              string
		authorizeResult   authorization.Result
		authorizeErr      error
		expectInnerCalled bool
		expectErr         error
	}{
		{name: "allow -> inner called", authorizeResult: authorization.Result{Decision: authorization.DecisionAllow}, expectInnerCalled: true},
		{name: "deny -> AccessDeniedError", authorizeResult: authorization.Result{Decision: authorization.DecisionDeny}, expectErr: errUnauthorized},
		{name: "authorizer error -> propagated", authorizeErr: errAuthorizerBoom, expectErr: errAuthorizerBoom},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			inner := handler.NewMockHandler(ctrl)
			authz := authorization.NewMockAuthorizer(ctrl)

			authz.EXPECT().
				Authorize(gomock.Any(), &authorization.Attributes{
					APIName:    "ForceResetNamespace",
					Namespace:  testNamespace,
					Permission: authorization.PermissionAdmin,
				}).
				Return(tc.authorizeResult, tc.authorizeErr).
				Times(1)

			if tc.expectInnerCalled {
				inner.EXPECT().
					ForceResetNamespace(gomock.Any(), &types.ForceResetNamespaceRequest{Namespace: testNamespace}).
					Return(&types.ForceResetNamespaceResponse{DeletedKeys: 5}, nil).
					Times(1)
			}

			resp, err := NewHandler(inner, authz).ForceResetNamespace(
				context.Background(),
				&types.ForceResetNamespaceRequest{Namespace: testNamespace},
			)

			if tc.expectErr != nil {
				assert.Nil(t, resp)
				assert.ErrorIs(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, int64(5), resp.DeletedKeys)
		})
	}
}

func TestAccessControlledHandler_DrainShards(t *testing.T) {
	tests := []struct {
		name              string
		authorizeResult   authorization.Result
		authorizeErr      error
		expectInnerCalled bool
		expectErr         error
	}{
		{name: "allow -> inner called", authorizeResult: authorization.Result{Decision: authorization.DecisionAllow}, expectInnerCalled: true},
		{name: "deny -> AccessDeniedError", authorizeResult: authorization.Result{Decision: authorization.DecisionDeny}, expectErr: errUnauthorized},
		{name: "authorizer error -> propagated", authorizeErr: errAuthorizerBoom, expectErr: errAuthorizerBoom},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			inner := handler.NewMockHandler(ctrl)
			authz := authorization.NewMockAuthorizer(ctrl)

			req := &types.DrainShardsRequest{Namespace: testNamespace, ShardKeys: []string{"shard-1"}}

			authz.EXPECT().
				Authorize(gomock.Any(), &authorization.Attributes{
					APIName:    "DrainShards",
					Namespace:  testNamespace,
					Permission: authorization.PermissionAdmin,
				}).
				Return(tc.authorizeResult, tc.authorizeErr).
				Times(1)

			if tc.expectInnerCalled {
				inner.EXPECT().
					DrainShards(gomock.Any(), req).
					Return(nil).
					Times(1)
			}

			err := NewHandler(inner, authz).DrainShards(context.Background(), req)

			if tc.expectErr != nil {
				assert.ErrorIs(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestAccessControlledHandler_UndrainShards(t *testing.T) {
	tests := []struct {
		name              string
		authorizeResult   authorization.Result
		authorizeErr      error
		expectInnerCalled bool
		expectErr         error
	}{
		{name: "allow -> inner called", authorizeResult: authorization.Result{Decision: authorization.DecisionAllow}, expectInnerCalled: true},
		{name: "deny -> AccessDeniedError", authorizeResult: authorization.Result{Decision: authorization.DecisionDeny}, expectErr: errUnauthorized},
		{name: "authorizer error -> propagated", authorizeErr: errAuthorizerBoom, expectErr: errAuthorizerBoom},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			inner := handler.NewMockHandler(ctrl)
			authz := authorization.NewMockAuthorizer(ctrl)

			req := &types.UndrainShardsRequest{Namespace: testNamespace, ShardKeys: []string{"shard-1"}}

			authz.EXPECT().
				Authorize(gomock.Any(), &authorization.Attributes{
					APIName:    "UndrainShards",
					Namespace:  testNamespace,
					Permission: authorization.PermissionAdmin,
				}).
				Return(tc.authorizeResult, tc.authorizeErr).
				Times(1)

			if tc.expectInnerCalled {
				inner.EXPECT().
					UndrainShards(gomock.Any(), req).
					Return(&types.UndrainShardsResponse{UndrainedShardKeys: []string{"shard-1"}}, nil).
					Times(1)
			}

			resp, err := NewHandler(inner, authz).UndrainShards(context.Background(), req)

			if tc.expectErr != nil {
				assert.Nil(t, resp)
				assert.ErrorIs(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, []string{"shard-1"}, resp.UndrainedShardKeys)
		})
	}
}

func TestAccessControlledHandler_GetDrainedShards(t *testing.T) {
	tests := []struct {
		name              string
		authorizeResult   authorization.Result
		authorizeErr      error
		expectInnerCalled bool
		expectErr         error
	}{
		{name: "allow -> inner called", authorizeResult: authorization.Result{Decision: authorization.DecisionAllow}, expectInnerCalled: true},
		{name: "deny -> AccessDeniedError", authorizeResult: authorization.Result{Decision: authorization.DecisionDeny}, expectErr: errUnauthorized},
		{name: "authorizer error -> propagated", authorizeErr: errAuthorizerBoom, expectErr: errAuthorizerBoom},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			inner := handler.NewMockHandler(ctrl)
			authz := authorization.NewMockAuthorizer(ctrl)

			req := &types.GetDrainedShardsRequest{Namespace: testNamespace}

			authz.EXPECT().
				Authorize(gomock.Any(), &authorization.Attributes{
					APIName:    "GetDrainedShards",
					Namespace:  testNamespace,
					Permission: authorization.PermissionRead,
				}).
				Return(tc.authorizeResult, tc.authorizeErr).
				Times(1)

			if tc.expectInnerCalled {
				inner.EXPECT().
					GetDrainedShards(gomock.Any(), req).
					Return(&types.GetDrainedShardsResponse{Namespace: testNamespace, ShardKeys: []string{"shard-1"}}, nil).
					Times(1)
			}

			resp, err := NewHandler(inner, authz).GetDrainedShards(context.Background(), req)

			if tc.expectErr != nil {
				assert.Nil(t, resp)
				assert.ErrorIs(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, testNamespace, resp.Namespace)
			assert.Equal(t, []string{"shard-1"}, resp.ShardKeys)
		})
	}
}

func TestAccessControlledHandler_ListNamespaces(t *testing.T) {
	tests := []struct {
		name              string
		authorizeResult   authorization.Result
		authorizeErr      error
		expectInnerCalled bool
		expectErr         error
	}{
		{name: "allow -> inner called", authorizeResult: authorization.Result{Decision: authorization.DecisionAllow}, expectInnerCalled: true},
		{name: "deny -> AccessDeniedError", authorizeResult: authorization.Result{Decision: authorization.DecisionDeny}, expectErr: errUnauthorized},
		{name: "authorizer error -> propagated", authorizeErr: errAuthorizerBoom, expectErr: errAuthorizerBoom},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			inner := handler.NewMockHandler(ctrl)
			authz := authorization.NewMockAuthorizer(ctrl)

			authz.EXPECT().
				Authorize(gomock.Any(), &authorization.Attributes{
					APIName:    "ListNamespaces",
					Namespace:  "",
					Permission: authorization.PermissionAdmin,
				}).
				Return(tc.authorizeResult, tc.authorizeErr).
				Times(1)

			if tc.expectInnerCalled {
				inner.EXPECT().
					ListNamespaces(gomock.Any(), gomock.Any()).
					Return(&types.ListNamespacesResponse{}, nil).
					Times(1)
			}

			resp, err := NewHandler(inner, authz).ListNamespaces(context.Background(), &types.ListNamespacesRequest{})

			if tc.expectErr != nil {
				assert.Nil(t, resp)
				assert.ErrorIs(t, err, tc.expectErr)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, resp)
		})
	}
}
