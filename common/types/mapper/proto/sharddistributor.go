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
	sharddistributorv1 "github.com/cadence-workflow/shard-manager/.gen/proto/sharddistributor/v1"
	"github.com/cadence-workflow/shard-manager/common/types"
)

// FromShardDistributorGetShardOwnerRequest converts a types.GetShardOwnerRequest to a sharddistributor.GetShardOwnerRequest
func FromShardDistributorGetShardOwnerRequest(t *types.GetShardOwnerRequest) *sharddistributorv1.GetShardOwnerRequest {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.GetShardOwnerRequest{
		ShardKey:  t.GetShardKey(),
		Namespace: t.GetNamespace(),
	}
}

// ToShardDistributorGetShardOwnerRequest converts a sharddistributor.GetShardOwnerRequest to a types.GetShardOwnerRequest
func ToShardDistributorGetShardOwnerRequest(t *sharddistributorv1.GetShardOwnerRequest) *types.GetShardOwnerRequest {
	if t == nil {
		return nil
	}
	return &types.GetShardOwnerRequest{
		ShardKey:  t.GetShardKey(),
		Namespace: t.GetNamespace(),
	}
}

// FromShardDistributorGetShardOwnerResponse converts a types.GetShardOwnerResponse to a sharddistributor.GetShardOwnerResponse
func FromShardDistributorGetShardOwnerResponse(t *types.GetShardOwnerResponse) *sharddistributorv1.GetShardOwnerResponse {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.GetShardOwnerResponse{
		Owner:     t.GetOwner(),
		Namespace: t.GetNamespace(),
		Metadata:  t.GetMetadata(),
	}
}

// ToShardDistributorGetShardOwnerResponse converts a sharddistributor.GetShardOwnerResponse to a types.GetShardOwnerResponse
func ToShardDistributorGetShardOwnerResponse(t *sharddistributorv1.GetShardOwnerResponse) *types.GetShardOwnerResponse {
	if t == nil {
		return nil
	}
	return &types.GetShardOwnerResponse{
		Owner:     t.GetOwner(),
		Namespace: t.GetNamespace(),
		Metadata:  t.GetMetadata(),
	}
}

// ToShardDistributorInspectShardRequest converts a sharddistributor.InspectShardRequest to a types.GetShardOwnerRequest.
func ToShardDistributorInspectShardRequest(t *sharddistributorv1.InspectShardRequest) *types.GetShardOwnerRequest {
	if t == nil {
		return nil
	}
	return &types.GetShardOwnerRequest{
		ShardKey:  t.GetShardKey(),
		Namespace: t.GetNamespace(),
	}
}

// FromShardDistributorInspectShardRequest converts a types.GetShardOwnerRequest to a sharddistributor.InspectShardRequest.
func FromShardDistributorInspectShardRequest(t *types.GetShardOwnerRequest) *sharddistributorv1.InspectShardRequest {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.InspectShardRequest{
		ShardKey:  t.GetShardKey(),
		Namespace: t.GetNamespace(),
	}
}

// FromShardDistributorInspectShardResponse converts a types.GetShardOwnerResponse to a sharddistributor.InspectShardResponse.
func FromShardDistributorInspectShardResponse(t *types.GetShardOwnerResponse) *sharddistributorv1.InspectShardResponse {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.InspectShardResponse{
		Owner:     t.GetOwner(),
		Namespace: t.GetNamespace(),
		Metadata:  t.GetMetadata(),
	}
}

// ToShardDistributorInspectShardResponse converts a sharddistributor.InspectShardResponse to a types.GetShardOwnerResponse.
func ToShardDistributorInspectShardResponse(t *sharddistributorv1.InspectShardResponse) *types.GetShardOwnerResponse {
	if t == nil {
		return nil
	}
	return &types.GetShardOwnerResponse{
		Owner:     t.GetOwner(),
		Namespace: t.GetNamespace(),
		Metadata:  t.GetMetadata(),
	}
}

func FromShardDistributorExecutorHeartbeatRequest(t *types.ExecutorHeartbeatRequest) *sharddistributorv1.HeartbeatRequest {
	if t == nil {
		return nil
	}

	// Convert the ExecutorStatus enum
	var status sharddistributorv1.ExecutorStatus
	switch t.GetStatus() {
	case types.ExecutorStatusINVALID:
		status = sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_INVALID
	case types.ExecutorStatusACTIVE:
		status = sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_ACTIVE
	case types.ExecutorStatusDRAINING:
		status = sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_DRAINING
	case types.ExecutorStatusDRAINED:
		status = sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_DRAINED
	default:
		status = sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_INVALID
	}

	// Convert the ShardStatusReports
	var shardStatusReports map[string]*sharddistributorv1.ShardStatusReport
	if t.GetShardStatusReports() != nil {
		shardStatusReports = make(map[string]*sharddistributorv1.ShardStatusReport)

		for shardKey, shardStatusReport := range t.GetShardStatusReports() {

			var status sharddistributorv1.ShardStatus
			switch shardStatusReport.GetStatus() {
			case types.ShardStatusINVALID:
				status = sharddistributorv1.ShardStatus_SHARD_STATUS_INVALID
			case types.ShardStatusREADY:
				status = sharddistributorv1.ShardStatus_SHARD_STATUS_READY
			case types.ShardStatusDONE:
				status = sharddistributorv1.ShardStatus_SHARD_STATUS_DONE
			default:
				status = sharddistributorv1.ShardStatus_SHARD_STATUS_INVALID
			}

			shardStatusReports[shardKey] = &sharddistributorv1.ShardStatusReport{
				Status:    status,
				ShardLoad: shardStatusReport.GetShardLoad(),
			}
		}
	}
	return &sharddistributorv1.HeartbeatRequest{
		Namespace:          t.GetNamespace(),
		ExecutorId:         t.GetExecutorID(),
		Status:             status,
		ShardStatusReports: shardStatusReports,
		Metadata:           t.GetMetadata(),
	}
}

func ToShardDistributorExecutorHeartbeatRequest(t *sharddistributorv1.HeartbeatRequest) *types.ExecutorHeartbeatRequest {
	if t == nil {
		return nil
	}

	// Convert the ExecutorStatus enum
	var status types.ExecutorStatus
	switch t.GetStatus() {
	case sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_INVALID:
		status = types.ExecutorStatusINVALID
	case sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_ACTIVE:
		status = types.ExecutorStatusACTIVE
	case sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_DRAINING:
		status = types.ExecutorStatusDRAINING
	case sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_DRAINED:
		status = types.ExecutorStatusDRAINED
	default:
		status = types.ExecutorStatusINVALID
	}

	// Convert the ShardStatusReports
	var shardStatusReports map[string]*types.ShardStatusReport
	if t.GetShardStatusReports() != nil {
		shardStatusReports = make(map[string]*types.ShardStatusReport)

		for shardKey, shardStatusReport := range t.GetShardStatusReports() {

			var status types.ShardStatus
			switch shardStatusReport.GetStatus() {
			case sharddistributorv1.ShardStatus_SHARD_STATUS_INVALID:
				status = types.ShardStatusINVALID
			case sharddistributorv1.ShardStatus_SHARD_STATUS_READY:
				status = types.ShardStatusREADY
			case sharddistributorv1.ShardStatus_SHARD_STATUS_DONE:
				status = types.ShardStatusDONE
			}

			shardStatusReports[shardKey] = &types.ShardStatusReport{
				Status:    status,
				ShardLoad: shardStatusReport.GetShardLoad(),
			}
		}
	}

	return &types.ExecutorHeartbeatRequest{
		Namespace:          t.GetNamespace(),
		ExecutorID:         t.GetExecutorId(),
		Status:             status,
		ShardStatusReports: shardStatusReports,
		Metadata:           t.GetMetadata(),
	}
}

func FromShardDistributorExecutorHeartbeatResponse(t *types.ExecutorHeartbeatResponse) *sharddistributorv1.HeartbeatResponse {
	if t == nil {
		return nil
	}

	// Convert the ShardAssignments
	var shardAssignments map[string]*sharddistributorv1.ShardAssignment
	migrationMode := toMigrationMode(t.GetMigrationMode())
	if t.GetShardAssignments() != nil {
		shardAssignments = make(map[string]*sharddistributorv1.ShardAssignment)

		for shardKey, shardAssignment := range t.GetShardAssignments() {
			var status sharddistributorv1.AssignmentStatus
			switch shardAssignment.GetStatus() {
			case types.AssignmentStatusINVALID:
				status = sharddistributorv1.AssignmentStatus_ASSIGNMENT_STATUS_INVALID
			case types.AssignmentStatusREADY:
				status = sharddistributorv1.AssignmentStatus_ASSIGNMENT_STATUS_READY
			}
			shardAssignments[shardKey] = &sharddistributorv1.ShardAssignment{
				Status: status,
			}
		}
	}

	return &sharddistributorv1.HeartbeatResponse{
		ShardAssignments: shardAssignments,
		MigrationMode:    migrationMode,
	}
}

func ToShardDistributorExecutorHeartbeatResponse(t *sharddistributorv1.HeartbeatResponse) *types.ExecutorHeartbeatResponse {
	if t == nil {
		return nil
	}

	// Convert the ShardAssignments
	var shardAssignments map[string]*types.ShardAssignment
	var migrationMode types.MigrationMode
	if t.GetShardAssignments() != nil {
		shardAssignments = make(map[string]*types.ShardAssignment)

		for shardKey, shardAssignment := range t.GetShardAssignments() {
			var status types.AssignmentStatus
			switch shardAssignment.GetStatus() {
			case sharddistributorv1.AssignmentStatus_ASSIGNMENT_STATUS_INVALID:
				status = types.AssignmentStatusINVALID
			case sharddistributorv1.AssignmentStatus_ASSIGNMENT_STATUS_READY:
				status = types.AssignmentStatusREADY
			}
			shardAssignments[shardKey] = &types.ShardAssignment{
				Status: status,
			}
		}
	}
	migrationMode = getMigrationModeFromProto(t.GetMigrationMode())

	return &types.ExecutorHeartbeatResponse{
		ShardAssignments: shardAssignments,
		MigrationMode:    migrationMode,
	}
}

func getMigrationModeFromProto(protoMigrationMode sharddistributorv1.MigrationMode) types.MigrationMode {
	var mode types.MigrationMode
	switch protoMigrationMode {
	case sharddistributorv1.MigrationMode_MIGRATION_MODE_LOCAL_PASSTHROUGH:
		mode = types.MigrationModeLOCALPASSTHROUGH
	case sharddistributorv1.MigrationMode_MIGRATION_MODE_ONBOARDED:
		mode = types.MigrationModeONBOARDED
	default:
		mode = types.MigrationModeINVALID
	}
	return mode
}

func toMigrationMode(modeSD types.MigrationMode) sharddistributorv1.MigrationMode {
	var mode sharddistributorv1.MigrationMode
	switch modeSD {
	case types.MigrationModeINVALID:
		mode = sharddistributorv1.MigrationMode_MIGRATION_MODE_INVALID
	case types.MigrationModeLOCALPASSTHROUGH:
		mode = sharddistributorv1.MigrationMode_MIGRATION_MODE_LOCAL_PASSTHROUGH
	case types.MigrationModeONBOARDED:
		mode = sharddistributorv1.MigrationMode_MIGRATION_MODE_ONBOARDED
	default:
		mode = sharddistributorv1.MigrationMode_MIGRATION_MODE_INVALID
	}
	return mode
}

// FromShardDistributorWatchNamespaceStateRequest converts a types.WatchNamespaceStateRequest to a sharddistributor.WatchNamespaceStateRequest
func FromShardDistributorWatchNamespaceStateRequest(t *types.WatchNamespaceStateRequest) *sharddistributorv1.WatchNamespaceStateRequest {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.WatchNamespaceStateRequest{
		Namespace: t.GetNamespace(),
	}
}

// ToShardDistributorWatchNamespaceStateRequest converts a sharddistributor.WatchNamespaceStateRequest to a types.WatchNamespaceStateRequest
func ToShardDistributorWatchNamespaceStateRequest(t *sharddistributorv1.WatchNamespaceStateRequest) *types.WatchNamespaceStateRequest {
	if t == nil {
		return nil
	}
	return &types.WatchNamespaceStateRequest{
		Namespace: t.GetNamespace(),
	}
}

// FromShardDistributorWatchNamespaceStateResponse converts a types.WatchNamespaceStateResponse to a sharddistributor.WatchNamespaceStateResponse
func FromShardDistributorWatchNamespaceStateResponse(t *types.WatchNamespaceStateResponse) *sharddistributorv1.WatchNamespaceStateResponse {
	if t == nil {
		return nil
	}

	var executors []*sharddistributorv1.ExecutorInfo

	for _, executor := range t.GetExecutors() {
		// Convert the Shards
		shards := make([]*sharddistributorv1.Shard, 0, len(executor.GetAssignedShards()))
		for _, shard := range executor.GetAssignedShards() {
			shards = append(shards, &sharddistributorv1.Shard{
				ShardKey: shard.GetShardKey(),
			})
		}
		executors = append(executors, &sharddistributorv1.ExecutorInfo{
			ExecutorId: executor.GetExecutorID(),
			Metadata:   executor.GetMetadata(),
			Shards:     shards,
		})
	}

	return &sharddistributorv1.WatchNamespaceStateResponse{
		Executors: executors,
	}
}

// ToShardDistributorWatchNamespaceStateResponse converts a sharddistributor.WatchNamespaceStateResponse to a types.WatchNamespaceStateResponse
func ToShardDistributorWatchNamespaceStateResponse(t *sharddistributorv1.WatchNamespaceStateResponse) *types.WatchNamespaceStateResponse {
	if t == nil {
		return nil
	}

	var executors []*types.ExecutorShardAssignment
	if t.GetExecutors() != nil {
		executors = make([]*types.ExecutorShardAssignment, 0, len(t.GetExecutors()))
		for _, executor := range t.GetExecutors() {
			// Convert the Shards
			shards := make([]*types.Shard, 0, len(executor.GetShards()))
			for _, shard := range executor.GetShards() {
				shards = append(shards, &types.Shard{
					ShardKey: shard.GetShardKey(),
				})
			}

			executors = append(executors, &types.ExecutorShardAssignment{
				ExecutorID:     executor.GetExecutorId(),
				Metadata:       executor.GetMetadata(),
				AssignedShards: shards,
			})
		}
	}

	return &types.WatchNamespaceStateResponse{
		Executors: executors,
	}
}

// FromShardDistributorGetNamespaceStateRequest converts a types.GetNamespaceStateRequest to a sharddistributor GetNamespaceStateRequest.
func FromShardDistributorGetNamespaceStateRequest(t *types.GetNamespaceStateRequest) *sharddistributorv1.GetNamespaceStateRequest {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.GetNamespaceStateRequest{
		Namespace: t.GetNamespace(),
	}
}

// ToShardDistributorGetNamespaceStateRequest converts a sharddistributor GetNamespaceStateRequest to a types.GetNamespaceStateRequest.
func ToShardDistributorGetNamespaceStateRequest(t *sharddistributorv1.GetNamespaceStateRequest) *types.GetNamespaceStateRequest {
	if t == nil {
		return nil
	}
	return &types.GetNamespaceStateRequest{
		Namespace: t.GetNamespace(),
	}
}

// FromShardDistributorGetNamespaceStateResponse converts a types.GetNamespaceStateResponse to a sharddistributor GetNamespaceStateResponse.
func FromShardDistributorGetNamespaceStateResponse(t *types.GetNamespaceStateResponse) *sharddistributorv1.GetNamespaceStateResponse {
	if t == nil {
		return nil
	}

	var executors []*sharddistributorv1.NamespaceExecutorState
	if t.GetExecutors() != nil {
		executors = make([]*sharddistributorv1.NamespaceExecutorState, 0, len(t.GetExecutors()))
		for _, ex := range t.GetExecutors() {
			executors = append(executors, fromShardDistributorNamespaceExecutorState(ex))
		}
	}

	return &sharddistributorv1.GetNamespaceStateResponse{
		Namespace: t.GetNamespace(),
		Executors: executors,
	}
}

// fromShardDistributorNamespaceExecutorState converts a types.NamespaceExecutorState to its proto counterpart.
func fromShardDistributorNamespaceExecutorState(ex *types.NamespaceExecutorState) *sharddistributorv1.NamespaceExecutorState {
	if ex == nil {
		return nil
	}

	var status sharddistributorv1.ExecutorStatus
	switch ex.GetStatus() {
	case types.ExecutorStatusINVALID:
		status = sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_INVALID
	case types.ExecutorStatusACTIVE:
		status = sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_ACTIVE
	case types.ExecutorStatusDRAINING:
		status = sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_DRAINING
	case types.ExecutorStatusDRAINED:
		status = sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_DRAINED
	default:
		status = sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_INVALID
	}

	var assigned []*sharddistributorv1.AssignedShardState
	if ex.GetAssignedShards() != nil {
		assigned = make([]*sharddistributorv1.AssignedShardState, 0, len(ex.GetAssignedShards()))
		for _, sh := range ex.GetAssignedShards() {
			var as sharddistributorv1.AssignmentStatus
			switch sh.GetAssignmentStatus() {
			case types.AssignmentStatusINVALID:
				as = sharddistributorv1.AssignmentStatus_ASSIGNMENT_STATUS_INVALID
			case types.AssignmentStatusREADY:
				as = sharddistributorv1.AssignmentStatus_ASSIGNMENT_STATUS_READY
			default:
				as = sharddistributorv1.AssignmentStatus_ASSIGNMENT_STATUS_INVALID
			}
			assigned = append(assigned, &sharddistributorv1.AssignedShardState{
				ShardKey:                 sh.GetShardKey(),
				AssignmentStatus:         as,
				AssignedStateModRevision: sh.GetAssignedStateModRevision(),
			})
		}
	}

	lastHB := ex.GetLastHeartbeat()
	return &sharddistributorv1.NamespaceExecutorState{
		ExecutorId:     ex.GetExecutorID(),
		Status:         status,
		LastHeartbeat:  timeToTimestamp(&lastHB),
		Metadata:       ex.GetMetadata(),
		AssignedShards: assigned,
	}
}

// ToShardDistributorGetNamespaceStateResponse converts a sharddistributor GetNamespaceStateResponse to a types.GetNamespaceStateResponse.
func ToShardDistributorGetNamespaceStateResponse(t *sharddistributorv1.GetNamespaceStateResponse) *types.GetNamespaceStateResponse {
	if t == nil {
		return nil
	}

	var executors []*types.NamespaceExecutorState
	if t.GetExecutors() != nil {
		executors = make([]*types.NamespaceExecutorState, 0, len(t.GetExecutors()))
		for _, ex := range t.GetExecutors() {
			executors = append(executors, toShardDistributorNamespaceExecutorState(ex))
		}
	}

	return &types.GetNamespaceStateResponse{
		Namespace: t.GetNamespace(),
		Executors: executors,
	}
}

// toShardDistributorNamespaceExecutorState converts a proto NamespaceExecutorState to its types counterpart.
func toShardDistributorNamespaceExecutorState(ex *sharddistributorv1.NamespaceExecutorState) *types.NamespaceExecutorState {
	if ex == nil {
		return nil
	}

	var status types.ExecutorStatus
	switch ex.GetStatus() {
	case sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_INVALID:
		status = types.ExecutorStatusINVALID
	case sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_ACTIVE:
		status = types.ExecutorStatusACTIVE
	case sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_DRAINING:
		status = types.ExecutorStatusDRAINING
	case sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_DRAINED:
		status = types.ExecutorStatusDRAINED
	default:
		status = types.ExecutorStatusINVALID
	}

	var assigned []*types.ExecutorAssignedShardState
	if ex.GetAssignedShards() != nil {
		assigned = make([]*types.ExecutorAssignedShardState, 0, len(ex.GetAssignedShards()))
		for _, sh := range ex.GetAssignedShards() {
			var as types.AssignmentStatus
			switch sh.GetAssignmentStatus() {
			case sharddistributorv1.AssignmentStatus_ASSIGNMENT_STATUS_INVALID:
				as = types.AssignmentStatusINVALID
			case sharddistributorv1.AssignmentStatus_ASSIGNMENT_STATUS_READY:
				as = types.AssignmentStatusREADY
			default:
				as = types.AssignmentStatusINVALID
			}
			assigned = append(assigned, &types.ExecutorAssignedShardState{
				ShardKey:                 sh.GetShardKey(),
				AssignmentStatus:         as,
				AssignedStateModRevision: sh.GetAssignedStateModRevision(),
			})
		}
	}

	lastHB := timestampToTimeVal(ex.GetLastHeartbeat())
	return &types.NamespaceExecutorState{
		ExecutorID:     ex.GetExecutorId(),
		Status:         status,
		LastHeartbeat:  lastHB,
		Metadata:       ex.GetMetadata(),
		AssignedShards: assigned,
	}
}

// FromShardDistributorGetExecutorStateRequest converts a types.GetExecutorStateRequest to a sharddistributor GetExecutorStateRequest.
func FromShardDistributorGetExecutorStateRequest(t *types.GetExecutorStateRequest) *sharddistributorv1.GetExecutorStateRequest {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.GetExecutorStateRequest{
		Namespace:  t.GetNamespace(),
		ExecutorId: t.GetExecutorID(),
	}
}

// ToShardDistributorGetExecutorStateRequest converts a sharddistributor GetExecutorStateRequest to a types.GetExecutorStateRequest.
func ToShardDistributorGetExecutorStateRequest(t *sharddistributorv1.GetExecutorStateRequest) *types.GetExecutorStateRequest {
	if t == nil {
		return nil
	}
	return &types.GetExecutorStateRequest{
		Namespace:  t.GetNamespace(),
		ExecutorID: t.GetExecutorId(),
	}
}

// FromShardDistributorGetExecutorStateResponse converts a types.GetExecutorStateResponse to a sharddistributor GetExecutorStateResponse.
func FromShardDistributorGetExecutorStateResponse(t *types.GetExecutorStateResponse) *sharddistributorv1.GetExecutorStateResponse {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.GetExecutorStateResponse{
		Namespace: t.GetNamespace(),
		Executor:  fromShardDistributorNamespaceExecutorState(t.GetExecutor()),
	}
}

// ToShardDistributorGetExecutorStateResponse converts a sharddistributor GetExecutorStateResponse to a types.GetExecutorStateResponse.
func ToShardDistributorGetExecutorStateResponse(t *sharddistributorv1.GetExecutorStateResponse) *types.GetExecutorStateResponse {
	if t == nil {
		return nil
	}
	return &types.GetExecutorStateResponse{
		Namespace: t.GetNamespace(),
		Executor:  toShardDistributorNamespaceExecutorState(t.GetExecutor()),
	}
}

// FromShardDistributorListNamespacesRequest converts a types.ListNamespacesRequest to a sharddistributor ListNamespacesRequest.
func FromShardDistributorListNamespacesRequest(t *types.ListNamespacesRequest) *sharddistributorv1.ListNamespacesRequest {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.ListNamespacesRequest{}
}

// ToShardDistributorListNamespacesRequest converts a sharddistributor ListNamespacesRequest to a types.ListNamespacesRequest.
func ToShardDistributorListNamespacesRequest(t *sharddistributorv1.ListNamespacesRequest) *types.ListNamespacesRequest {
	if t == nil {
		return nil
	}
	return &types.ListNamespacesRequest{}
}

// FromShardDistributorListNamespacesResponse converts a types.ListNamespacesResponse to a sharddistributor ListNamespacesResponse.
func FromShardDistributorListNamespacesResponse(t *types.ListNamespacesResponse) *sharddistributorv1.ListNamespacesResponse {
	if t == nil {
		return nil
	}
	var namespaces []*sharddistributorv1.NamespaceConfig
	if t.GetNamespaces() != nil {
		namespaces = make([]*sharddistributorv1.NamespaceConfig, 0, len(t.GetNamespaces()))
		for _, ns := range t.GetNamespaces() {
			namespaces = append(namespaces, fromShardDistributorNamespaceConfig(ns))
		}
	}
	return &sharddistributorv1.ListNamespacesResponse{
		Namespaces: namespaces,
	}
}

// ToShardDistributorListNamespacesResponse converts a sharddistributor ListNamespacesResponse to a types.ListNamespacesResponse.
func ToShardDistributorListNamespacesResponse(t *sharddistributorv1.ListNamespacesResponse) *types.ListNamespacesResponse {
	if t == nil {
		return nil
	}
	var namespaces []*types.NamespaceConfig
	if t.GetNamespaces() != nil {
		namespaces = make([]*types.NamespaceConfig, 0, len(t.GetNamespaces()))
		for _, ns := range t.GetNamespaces() {
			namespaces = append(namespaces, toShardDistributorNamespaceConfig(ns))
		}
	}
	return &types.ListNamespacesResponse{
		Namespaces: namespaces,
	}
}

func fromShardDistributorNamespaceConfig(t *types.NamespaceConfig) *sharddistributorv1.NamespaceConfig {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.NamespaceConfig{
		Name:     t.GetName(),
		Type:     t.GetType(),
		Mode:     t.GetMode(),
		ShardNum: t.GetShardNum(),
	}
}

func toShardDistributorNamespaceConfig(t *sharddistributorv1.NamespaceConfig) *types.NamespaceConfig {
	if t == nil {
		return nil
	}
	return &types.NamespaceConfig{
		Name:     t.GetName(),
		Type:     t.GetType(),
		Mode:     t.GetMode(),
		ShardNum: t.GetShardNum(),
	}
}

// FromShardDistributorDrainShardsRequest converts a types.DrainShardsRequest to its proto counterpart.
func FromShardDistributorDrainShardsRequest(t *types.DrainShardsRequest) *sharddistributorv1.DrainShardsRequest {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.DrainShardsRequest{
		Namespace: t.GetNamespace(),
		ShardKeys: t.GetShardKeys(),
	}
}

// ToShardDistributorDrainShardsRequest converts a proto DrainShardsRequest to its types counterpart.
func ToShardDistributorDrainShardsRequest(t *sharddistributorv1.DrainShardsRequest) *types.DrainShardsRequest {
	if t == nil {
		return nil
	}
	return &types.DrainShardsRequest{
		Namespace: t.GetNamespace(),
		ShardKeys: t.GetShardKeys(),
	}
}

// FromShardDistributorDrainShardsResponse converts a types.DrainShardsResponse to its proto counterpart.
func FromShardDistributorDrainShardsResponse(t *types.DrainShardsResponse) *sharddistributorv1.DrainShardsResponse {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.DrainShardsResponse{
		DrainedShardKeys: t.GetDrainedShardKeys(),
	}
}

// ToShardDistributorDrainShardsResponse converts a proto DrainShardsResponse to its types counterpart.
func ToShardDistributorDrainShardsResponse(t *sharddistributorv1.DrainShardsResponse) *types.DrainShardsResponse {
	if t == nil {
		return nil
	}
	return &types.DrainShardsResponse{
		DrainedShardKeys: t.GetDrainedShardKeys(),
	}
}

// FromShardDistributorUndrainShardsRequest converts a types.UndrainShardsRequest to its proto counterpart.
func FromShardDistributorUndrainShardsRequest(t *types.UndrainShardsRequest) *sharddistributorv1.UndrainShardsRequest {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.UndrainShardsRequest{
		Namespace: t.GetNamespace(),
		ShardKeys: t.GetShardKeys(),
	}
}

// ToShardDistributorUndrainShardsRequest converts a proto UndrainShardsRequest to its types counterpart.
func ToShardDistributorUndrainShardsRequest(t *sharddistributorv1.UndrainShardsRequest) *types.UndrainShardsRequest {
	if t == nil {
		return nil
	}
	return &types.UndrainShardsRequest{
		Namespace: t.GetNamespace(),
		ShardKeys: t.GetShardKeys(),
	}
}

// FromShardDistributorUndrainShardsResponse converts a types.UndrainShardsResponse to its proto counterpart.
func FromShardDistributorUndrainShardsResponse(t *types.UndrainShardsResponse) *sharddistributorv1.UndrainShardsResponse {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.UndrainShardsResponse{
		UndrainedShardKeys: t.GetUndrainedShardKeys(),
	}
}

// ToShardDistributorUndrainShardsResponse converts a proto UndrainShardsResponse to its types counterpart.
func ToShardDistributorUndrainShardsResponse(t *sharddistributorv1.UndrainShardsResponse) *types.UndrainShardsResponse {
	if t == nil {
		return nil
	}
	return &types.UndrainShardsResponse{
		UndrainedShardKeys: t.GetUndrainedShardKeys(),
	}
}

// FromShardDistributorGetDrainedShardsRequest converts a types.GetDrainedShardsRequest to its proto counterpart.
func FromShardDistributorGetDrainedShardsRequest(t *types.GetDrainedShardsRequest) *sharddistributorv1.GetDrainedShardsRequest {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.GetDrainedShardsRequest{
		Namespace: t.GetNamespace(),
	}
}

// ToShardDistributorGetDrainedShardsRequest converts a proto GetDrainedShardsRequest to its types counterpart.
func ToShardDistributorGetDrainedShardsRequest(t *sharddistributorv1.GetDrainedShardsRequest) *types.GetDrainedShardsRequest {
	if t == nil {
		return nil
	}
	return &types.GetDrainedShardsRequest{
		Namespace: t.GetNamespace(),
	}
}

// FromShardDistributorGetDrainedShardsResponse converts a types.GetDrainedShardsResponse to its proto counterpart.
func FromShardDistributorGetDrainedShardsResponse(t *types.GetDrainedShardsResponse) *sharddistributorv1.GetDrainedShardsResponse {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.GetDrainedShardsResponse{
		Namespace: t.GetNamespace(),
		ShardKeys: t.GetShardKeys(),
	}
}

// ToShardDistributorGetDrainedShardsResponse converts a proto GetDrainedShardsResponse to its types counterpart.
func ToShardDistributorGetDrainedShardsResponse(t *sharddistributorv1.GetDrainedShardsResponse) *types.GetDrainedShardsResponse {
	if t == nil {
		return nil
	}
	return &types.GetDrainedShardsResponse{
		Namespace: t.GetNamespace(),
		ShardKeys: t.GetShardKeys(),
	}
}

// FromShardDistributorForceResetNamespaceRequest converts a types.ForceResetNamespaceRequest to a sharddistributor ForceResetNamespaceRequest.
func FromShardDistributorForceResetNamespaceRequest(t *types.ForceResetNamespaceRequest) *sharddistributorv1.ForceResetNamespaceRequest {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.ForceResetNamespaceRequest{
		Namespace: t.GetNamespace(),
	}
}

// ToShardDistributorForceResetNamespaceRequest converts a sharddistributor ForceResetNamespaceRequest to a types.ForceResetNamespaceRequest.
func ToShardDistributorForceResetNamespaceRequest(t *sharddistributorv1.ForceResetNamespaceRequest) *types.ForceResetNamespaceRequest {
	if t == nil {
		return nil
	}
	return &types.ForceResetNamespaceRequest{
		Namespace: t.GetNamespace(),
	}
}

// FromShardDistributorForceResetNamespaceResponse converts a types.ForceResetNamespaceResponse to a sharddistributor ForceResetNamespaceResponse.
func FromShardDistributorForceResetNamespaceResponse(t *types.ForceResetNamespaceResponse) *sharddistributorv1.ForceResetNamespaceResponse {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.ForceResetNamespaceResponse{
		DeletedKeys: t.GetDeletedKeys(),
	}
}

// ToShardDistributorForceResetNamespaceResponse converts a sharddistributor ForceResetNamespaceResponse to a types.ForceResetNamespaceResponse.
func ToShardDistributorForceResetNamespaceResponse(t *sharddistributorv1.ForceResetNamespaceResponse) *types.ForceResetNamespaceResponse {
	if t == nil {
		return nil
	}
	return &types.ForceResetNamespaceResponse{
		DeletedKeys: t.GetDeletedKeys(),
	}
}
