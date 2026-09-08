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

package proto

import (
	"testing"

	fuzz "github.com/google/gofuzz"
	"github.com/stretchr/testify/assert"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/common/types/mapper/testutils"
	"github.com/cadence-workflow/shard-manager/common/types/testdata"
)

func TestFromShardDistributorGetShardOwnerRequest(t *testing.T) {
	for _, item := range []*types.GetShardOwnerRequest{nil, {}, &testdata.ShardDistributorGetShardOwnerRequest} {
		assert.Equal(t, item, ToShardDistributorGetShardOwnerRequest(FromShardDistributorGetShardOwnerRequest(item)))
	}
}

func TestFromShardDistributorGetShardOwnerResponse(t *testing.T) {
	for _, item := range []*types.GetShardOwnerResponse{nil, {}, &testdata.ShardDistributorGetShardOwnerResponse} {
		assert.Equal(t, item, ToShardDistributorGetShardOwnerResponse(FromShardDistributorGetShardOwnerResponse(item)))
	}
}

func TestFromShardDistributorInspectShardRequest(t *testing.T) {
	for _, item := range []*types.GetShardOwnerRequest{nil, {}, &testdata.ShardDistributorGetShardOwnerRequest} {
		assert.Equal(t, item, ToShardDistributorInspectShardRequest(FromShardDistributorInspectShardRequest(item)))
	}
}

func TestFromShardDistributorInspectShardResponse(t *testing.T) {
	for _, item := range []*types.GetShardOwnerResponse{nil, {}, &testdata.ShardDistributorGetShardOwnerResponse} {
		assert.Equal(t, item, ToShardDistributorInspectShardResponse(FromShardDistributorInspectShardResponse(item)))
	}
}

func TestFromShardDistributorExecutorHeartbeatRequest(t *testing.T) {
	for _, item := range []*types.ExecutorHeartbeatRequest{nil, {}, &testdata.ShardDistributorExecutorHeartbeatRequest} {
		assert.Equal(t, item, ToShardDistributorExecutorHeartbeatRequest(FromShardDistributorExecutorHeartbeatRequest(item)))
	}
}

func TestToShardDistributorExecutorHeartbeatResponse(t *testing.T) {
	for _, item := range []*types.ExecutorHeartbeatResponse{nil, {}, &testdata.ShardDistributorExecutorHeartbeatResponse} {
		assert.Equal(t, item, ToShardDistributorExecutorHeartbeatResponse(FromShardDistributorExecutorHeartbeatResponse(item)))
	}
}

func TestFromShardDistributorWatchNamespaceStateRequest(t *testing.T) {
	for _, item := range []*types.WatchNamespaceStateRequest{nil, {}, &testdata.ShardDistributorWatchNamespaceStateRequest} {
		assert.Equal(t, item, ToShardDistributorWatchNamespaceStateRequest(FromShardDistributorWatchNamespaceStateRequest(item)))
	}
}

func TestFromShardDistributorWatchNamespaceStateResponse(t *testing.T) {
	for _, item := range []*types.WatchNamespaceStateResponse{nil, {}, &testdata.ShardDistributorWatchNamespaceStateResponse} {
		assert.Equal(t, item, ToShardDistributorWatchNamespaceStateResponse(FromShardDistributorWatchNamespaceStateResponse(item)))
	}
}

func TestFromShardDistributorGetNamespaceStateRequest(t *testing.T) {
	for _, item := range []*types.GetNamespaceStateRequest{nil, {}, &testdata.ShardDistributorGetNamespaceStateRequest} {
		assert.Equal(t, item, ToShardDistributorGetNamespaceStateRequest(FromShardDistributorGetNamespaceStateRequest(item)))
	}
}

func TestFromShardDistributorGetNamespaceStateResponse(t *testing.T) {
	for _, item := range []*types.GetNamespaceStateResponse{nil, {}, &testdata.ShardDistributorGetNamespaceStateResponse} {
		assert.Equal(t, item, ToShardDistributorGetNamespaceStateResponse(FromShardDistributorGetNamespaceStateResponse(item)))
	}
}

func TestFromShardDistributorGetExecutorStateRequest(t *testing.T) {
	for _, item := range []*types.GetExecutorStateRequest{nil, {}, &testdata.ShardDistributorGetExecutorStateRequest} {
		assert.Equal(t, item, ToShardDistributorGetExecutorStateRequest(FromShardDistributorGetExecutorStateRequest(item)))
	}
}

func TestFromShardDistributorGetExecutorStateResponse(t *testing.T) {
	for _, item := range []*types.GetExecutorStateResponse{nil, {}, &testdata.ShardDistributorGetExecutorStateResponse} {
		assert.Equal(t, item, ToShardDistributorGetExecutorStateResponse(FromShardDistributorGetExecutorStateResponse(item)))
	}
}

func TestFromShardDistributorListNamespacesRequest(t *testing.T) {
	for _, item := range []*types.ListNamespacesRequest{nil, {}, &testdata.ShardDistributorListNamespacesRequest} {
		assert.Equal(t, item, ToShardDistributorListNamespacesRequest(FromShardDistributorListNamespacesRequest(item)))
	}
}

func TestFromShardDistributorListNamespacesResponse(t *testing.T) {
	for _, item := range []*types.ListNamespacesResponse{nil, {}, &testdata.ShardDistributorListNamespacesResponse} {
		assert.Equal(t, item, ToShardDistributorListNamespacesResponse(FromShardDistributorListNamespacesResponse(item)))
	}
}

func TestFromShardDistributorForceResetNamespaceRequest(t *testing.T) {
	for _, item := range []*types.ForceResetNamespaceRequest{nil, {}, &testdata.ShardDistributorForceResetNamespaceRequest} {
		assert.Equal(t, item, ToShardDistributorForceResetNamespaceRequest(FromShardDistributorForceResetNamespaceRequest(item)))
	}
}

func TestFromShardDistributorForceResetNamespaceResponse(t *testing.T) {
	for _, item := range []*types.ForceResetNamespaceResponse{nil, {}, &testdata.ShardDistributorForceResetNamespaceResponse} {
		assert.Equal(t, item, ToShardDistributorForceResetNamespaceResponse(FromShardDistributorForceResetNamespaceResponse(item)))
	}
}

// --- Fuzz tests for sharddistributor mapper functions ---

// ExecutorStatusFuzzer generates valid ExecutorStatus enum values (0-3: INVALID, ACTIVE, DRAINING, DRAINED).
func ExecutorStatusFuzzer(e *types.ExecutorStatus, c fuzz.Continue) {
	*e = types.ExecutorStatus(c.Intn(5)) // 0-4
}

// ShardStatusFuzzer generates valid ShardStatus enum values (0-2: INVALID, READY, DONE).
func ShardStatusFuzzer(e *types.ShardStatus, c fuzz.Continue) {
	*e = types.ShardStatus(c.Intn(3)) // 0-2
}

// AssignmentStatusFuzzer generates valid AssignmentStatus enum values (0-1: INVALID, READY).
func AssignmentStatusFuzzer(e *types.AssignmentStatus, c fuzz.Continue) {
	*e = types.AssignmentStatus(c.Intn(2)) // 0-1
}

// MigrationModeFuzzer generates valid MigrationMode enum values
// (0-2: INVALID, LOCAL_PASSTHROUGH, ONBOARDED).
func MigrationModeFuzzer(e *types.MigrationMode, c fuzz.Continue) {
	*e = types.MigrationMode(c.Intn(3)) // 0-2
}

// ExecutorHeartbeatRequestFuzzer avoids nil map values: the mapper constructs a new
// struct from nil-safe getters, so nil and &ShardStatusReport{} round-trip identically.
func ExecutorHeartbeatRequestFuzzer(r *types.ExecutorHeartbeatRequest, c fuzz.Continue) {
	c.FuzzNoCustom(r)
	for k, v := range r.ShardStatusReports {
		if v == nil {
			r.ShardStatusReports[k] = &types.ShardStatusReport{}
		}
	}
}

// ExecutorHeartbeatResponseFuzzer avoids nil map values: the mapper constructs a new
// struct from nil-safe getters, so nil and &ShardAssignment{} round-trip identically.
func ExecutorHeartbeatResponseFuzzer(r *types.ExecutorHeartbeatResponse, c fuzz.Continue) {
	c.FuzzNoCustom(r)
	for k, v := range r.ShardAssignments {
		if v == nil {
			r.ShardAssignments[k] = &types.ShardAssignment{}
		}
	}
}

// WatchNamespaceStateResponseFuzzer avoids nil slices/elements: protobuf repeated
// fields don't distinguish nil from empty, so nil round-trips to [] via make().
func WatchNamespaceStateResponseFuzzer(r *types.WatchNamespaceStateResponse, c fuzz.Continue) {
	c.FuzzNoCustom(r)
	for i, executor := range r.Executors {
		if executor == nil {
			r.Executors[i] = &types.ExecutorShardAssignment{
				AssignedShards: []*types.Shard{},
			}
		} else {
			// nil AssignedShards becomes [] after round-trip (protobuf nil/empty equivalence).
			if executor.AssignedShards == nil {
				executor.AssignedShards = []*types.Shard{}
			}
			for j, shard := range executor.AssignedShards {
				if shard == nil {
					executor.AssignedShards[j] = &types.Shard{}
				}
			}
		}
	}
}

// GetNamespaceStateResponseFuzzer avoids nil slices/elements for protobuf repeated fields.
func GetNamespaceStateResponseFuzzer(r *types.GetNamespaceStateResponse, c fuzz.Continue) {
	c.FuzzNoCustom(r)
	for i, ex := range r.Executors {
		if ex == nil {
			r.Executors[i] = &types.NamespaceExecutorState{
				AssignedShards: []*types.ExecutorAssignedShardState{},
			}
		} else {
			if ex.AssignedShards == nil {
				ex.AssignedShards = []*types.ExecutorAssignedShardState{}
			}
			for j, sh := range ex.AssignedShards {
				if sh == nil {
					ex.AssignedShards[j] = &types.ExecutorAssignedShardState{}
				}
			}
		}
	}
}

// GetExecutorStateResponseFuzzer avoids nil slices/elements for protobuf repeated fields.
func GetExecutorStateResponseFuzzer(r *types.GetExecutorStateResponse, c fuzz.Continue) {
	c.FuzzNoCustom(r)
	if r.Executor == nil {
		return
	}
	if r.Executor.AssignedShards == nil {
		r.Executor.AssignedShards = []*types.ExecutorAssignedShardState{}
	}
	for j, sh := range r.Executor.AssignedShards {
		if sh == nil {
			r.Executor.AssignedShards[j] = &types.ExecutorAssignedShardState{}
		}
	}
}

func TestGetShardOwnerRequestFuzz(t *testing.T) {
	testutils.RunMapperFuzzTest(t, FromShardDistributorGetShardOwnerRequest, ToShardDistributorGetShardOwnerRequest)
}

func TestGetShardOwnerResponseFuzz(t *testing.T) {
	testutils.RunMapperFuzzTest(t, FromShardDistributorGetShardOwnerResponse, ToShardDistributorGetShardOwnerResponse)
}

func TestInspectShardRequestFuzz(t *testing.T) {
	testutils.RunMapperFuzzTest(t, FromShardDistributorInspectShardRequest, ToShardDistributorInspectShardRequest)
}

func TestInspectShardResponseFuzz(t *testing.T) {
	testutils.RunMapperFuzzTest(t, FromShardDistributorInspectShardResponse, ToShardDistributorInspectShardResponse)
}

func TestWatchNamespaceStateRequestFuzz(t *testing.T) {
	testutils.RunMapperFuzzTest(t, FromShardDistributorWatchNamespaceStateRequest, ToShardDistributorWatchNamespaceStateRequest)
}

func TestWatchNamespaceStateResponseFuzz(t *testing.T) {
	// WatchNamespaceStateResponseFuzzer avoids nil slices/elements because protobuf
	// cannot distinguish nil from empty repeated fields — both round-trip to non-nil [].
	testutils.RunMapperFuzzTest(t, FromShardDistributorWatchNamespaceStateResponse, ToShardDistributorWatchNamespaceStateResponse,
		testutils.WithCustomFuncs(WatchNamespaceStateResponseFuzzer),
	)
}

func TestGetNamespaceStateRequestFuzz(t *testing.T) {
	testutils.RunMapperFuzzTest(t, FromShardDistributorGetNamespaceStateRequest, ToShardDistributorGetNamespaceStateRequest)
}

func TestGetExecutorStateRequestFuzz(t *testing.T) {
	testutils.RunMapperFuzzTest(t, FromShardDistributorGetExecutorStateRequest, ToShardDistributorGetExecutorStateRequest)
}

func TestGetExecutorStateResponseFuzz(t *testing.T) {
	testutils.RunMapperFuzzTest(t, FromShardDistributorGetExecutorStateResponse, ToShardDistributorGetExecutorStateResponse,
		testutils.WithCustomFuncs(
			ExecutorStatusFuzzer,
			AssignmentStatusFuzzer,
			GetExecutorStateResponseFuzzer,
		),
	)
}

func TestGetNamespaceStateResponseFuzz(t *testing.T) {
	testutils.RunMapperFuzzTest(t, FromShardDistributorGetNamespaceStateResponse, ToShardDistributorGetNamespaceStateResponse,
		testutils.WithCustomFuncs(
			ExecutorStatusFuzzer,
			AssignmentStatusFuzzer,
			GetNamespaceStateResponseFuzzer,
		),
	)
}

func TestExecutorHeartbeatRequestFuzz(t *testing.T) {
	testutils.RunMapperFuzzTest(t, FromShardDistributorExecutorHeartbeatRequest, ToShardDistributorExecutorHeartbeatRequest,
		testutils.WithCustomFuncs(ExecutorStatusFuzzer, ShardStatusFuzzer, ExecutorHeartbeatRequestFuzzer),
	)
}

func TestExecutorHeartbeatResponseFuzz(t *testing.T) {
	testutils.RunMapperFuzzTest(t, FromShardDistributorExecutorHeartbeatResponse, ToShardDistributorExecutorHeartbeatResponse,
		testutils.WithCustomFuncs(AssignmentStatusFuzzer, MigrationModeFuzzer, ExecutorHeartbeatResponseFuzzer),
	)
}

func TestListNamespacesRequestFuzz(t *testing.T) {
	testutils.RunMapperFuzzTest(t, FromShardDistributorListNamespacesRequest, ToShardDistributorListNamespacesRequest)
}

// ListNamespacesResponseFuzzer normalizes nil slice elements to zero-value structs.
// protobuf repeated fields don't distinguish nil from empty for inner pointers, so
// nil entries round-trip to non-nil &NamespaceConfig{} via the per-element mapper.
func ListNamespacesResponseFuzzer(r *types.ListNamespacesResponse, c fuzz.Continue) {
	c.FuzzNoCustom(r)
	for i, ns := range r.Namespaces {
		if ns == nil {
			r.Namespaces[i] = &types.NamespaceConfig{}
		}
	}
}

func TestListNamespacesResponseFuzz(t *testing.T) {
	testutils.RunMapperFuzzTest(t, FromShardDistributorListNamespacesResponse, ToShardDistributorListNamespacesResponse,
		testutils.WithCustomFuncs(ListNamespacesResponseFuzzer),
	)
}

func TestForceResetNamespaceRequestFuzz(t *testing.T) {
	testutils.RunMapperFuzzTest(t, FromShardDistributorForceResetNamespaceRequest, ToShardDistributorForceResetNamespaceRequest)
}

func TestForceResetNamespaceResponseFuzz(t *testing.T) {
	testutils.RunMapperFuzzTest(t, FromShardDistributorForceResetNamespaceResponse, ToShardDistributorForceResetNamespaceResponse)
}
