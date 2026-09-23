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

	status := fromShardDistributorExecutorStatus(t.GetStatus())
	shardStatusReports := fromShardDistributorShardStatusReports(t.GetShardStatusReports())

	return &sharddistributorv1.HeartbeatRequest{
		Namespace:          t.GetNamespace(),
		ExecutorId:         t.GetExecutorID(),
		Status:             status,
		ShardStatusReports: shardStatusReports,
		Metadata:           t.GetMetadata(),
		HostMetadata:       fromShardDistributorHostMetadata(t.GetHostMetadata()),
	}
}

func ToShardDistributorExecutorHeartbeatRequest(t *sharddistributorv1.HeartbeatRequest) *types.ExecutorHeartbeatRequest {
	if t == nil {
		return nil
	}

	status := toShardDistributorExecutorStatus(t.GetStatus())
	shardStatusReports := toShardDistributorShardStatusReports(t.GetShardStatusReports())

	return &types.ExecutorHeartbeatRequest{
		Namespace:          t.GetNamespace(),
		ExecutorID:         t.GetExecutorId(),
		Status:             status,
		ShardStatusReports: shardStatusReports,
		Metadata:           t.GetMetadata(),
		HostMetadata:       toShardDistributorHostMetadata(t.GetHostMetadata()),
	}
}

func fromShardDistributorHostMetadata(t *types.HostMetadata) *sharddistributorv1.HostMetadata {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.HostMetadata{
		HostName: t.GetHostName(),
	}
}

func toShardDistributorHostMetadata(t *sharddistributorv1.HostMetadata) *types.HostMetadata {
	if t == nil {
		return nil
	}
	return &types.HostMetadata{
		HostName: t.GetHostName(),
	}
}

func FromShardDistributorExecutorHeartbeatResponse(t *types.ExecutorHeartbeatResponse) *sharddistributorv1.HeartbeatResponse {
	if t == nil {
		return nil
	}

	shardAssignments := fromShardDistributorShardAssignments(t.GetShardAssignments())
	migrationMode := fromShardDistributorMigrationMode(t.GetMigrationMode())

	return &sharddistributorv1.HeartbeatResponse{
		ShardAssignments: shardAssignments,
		MigrationMode:    migrationMode,
	}
}

func ToShardDistributorExecutorHeartbeatResponse(t *sharddistributorv1.HeartbeatResponse) *types.ExecutorHeartbeatResponse {
	if t == nil {
		return nil
	}

	shardAssignments := toShardDistributorShardAssignments(t.GetShardAssignments())
	migrationMode := toShardDistributorMigrationMode(t.GetMigrationMode())

	return &types.ExecutorHeartbeatResponse{
		ShardAssignments: shardAssignments,
		MigrationMode:    migrationMode,
	}
}

func fromShardDistributorMigrationMode(mode types.MigrationMode) sharddistributorv1.MigrationMode {
	var result sharddistributorv1.MigrationMode
	switch mode {
	case types.MigrationModeINVALID:
		result = sharddistributorv1.MigrationMode_MIGRATION_MODE_INVALID
	case types.MigrationModeLOCALPASSTHROUGH:
		result = sharddistributorv1.MigrationMode_MIGRATION_MODE_LOCAL_PASSTHROUGH
	case types.MigrationModeONBOARDED:
		result = sharddistributorv1.MigrationMode_MIGRATION_MODE_ONBOARDED
	default:
		result = sharddistributorv1.MigrationMode_MIGRATION_MODE_INVALID
	}
	return result
}

func toShardDistributorMigrationMode(mode sharddistributorv1.MigrationMode) types.MigrationMode {
	var result types.MigrationMode
	switch mode {
	case sharddistributorv1.MigrationMode_MIGRATION_MODE_LOCAL_PASSTHROUGH:
		result = types.MigrationModeLOCALPASSTHROUGH
	case sharddistributorv1.MigrationMode_MIGRATION_MODE_ONBOARDED:
		result = types.MigrationModeONBOARDED
	default:
		result = types.MigrationModeINVALID
	}
	return result
}

func fromShardDistributorExecutorStatus(status types.ExecutorStatus) sharddistributorv1.ExecutorStatus {
	var result sharddistributorv1.ExecutorStatus
	switch status {
	case types.ExecutorStatusINVALID:
		result = sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_INVALID
	case types.ExecutorStatusACTIVE:
		result = sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_ACTIVE
	case types.ExecutorStatusDRAINING:
		result = sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_DRAINING
	case types.ExecutorStatusDRAINED:
		result = sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_DRAINED
	default:
		result = sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_INVALID
	}
	return result
}

func toShardDistributorExecutorStatus(status sharddistributorv1.ExecutorStatus) types.ExecutorStatus {
	var result types.ExecutorStatus
	switch status {
	case sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_ACTIVE:
		result = types.ExecutorStatusACTIVE
	case sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_DRAINING:
		result = types.ExecutorStatusDRAINING
	case sharddistributorv1.ExecutorStatus_EXECUTOR_STATUS_DRAINED:
		result = types.ExecutorStatusDRAINED
	default:
		result = types.ExecutorStatusINVALID
	}
	return result
}

func fromShardDistributorShardStatus(status types.ShardStatus) sharddistributorv1.ShardStatus {
	var result sharddistributorv1.ShardStatus
	switch status {
	case types.ShardStatusINVALID:
		result = sharddistributorv1.ShardStatus_SHARD_STATUS_INVALID
	case types.ShardStatusREADY:
		result = sharddistributorv1.ShardStatus_SHARD_STATUS_READY
	case types.ShardStatusDONE:
		result = sharddistributorv1.ShardStatus_SHARD_STATUS_DONE
	default:
		result = sharddistributorv1.ShardStatus_SHARD_STATUS_INVALID
	}
	return result
}

func toShardDistributorShardStatus(status sharddistributorv1.ShardStatus) types.ShardStatus {
	var result types.ShardStatus
	switch status {
	case sharddistributorv1.ShardStatus_SHARD_STATUS_READY:
		result = types.ShardStatusREADY
	case sharddistributorv1.ShardStatus_SHARD_STATUS_DONE:
		result = types.ShardStatusDONE
	default:
		result = types.ShardStatusINVALID
	}
	return result
}

func fromShardDistributorAssignmentStatus(status types.AssignmentStatus) sharddistributorv1.AssignmentStatus {
	var result sharddistributorv1.AssignmentStatus
	switch status {
	case types.AssignmentStatusINVALID:
		result = sharddistributorv1.AssignmentStatus_ASSIGNMENT_STATUS_INVALID
	case types.AssignmentStatusREADY:
		result = sharddistributorv1.AssignmentStatus_ASSIGNMENT_STATUS_READY
	default:
		result = sharddistributorv1.AssignmentStatus_ASSIGNMENT_STATUS_INVALID
	}
	return result
}

func toShardDistributorAssignmentStatus(status sharddistributorv1.AssignmentStatus) types.AssignmentStatus {
	var result types.AssignmentStatus
	switch status {
	case sharddistributorv1.AssignmentStatus_ASSIGNMENT_STATUS_READY:
		result = types.AssignmentStatusREADY
	default:
		result = types.AssignmentStatusINVALID
	}
	return result
}

func fromShardDistributorShardStatusReports(reports map[string]*types.ShardStatusReport) map[string]*sharddistributorv1.ShardStatusReport {
	if reports == nil {
		return nil
	}

	result := make(map[string]*sharddistributorv1.ShardStatusReport, len(reports))
	for shardKey, report := range reports {
		status := fromShardDistributorShardStatus(report.GetStatus())
		result[shardKey] = &sharddistributorv1.ShardStatusReport{
			Status:    status,
			ShardLoad: report.GetShardLoad(),
		}
	}
	return result
}

func toShardDistributorShardStatusReports(reports map[string]*sharddistributorv1.ShardStatusReport) map[string]*types.ShardStatusReport {
	if reports == nil {
		return nil
	}

	result := make(map[string]*types.ShardStatusReport, len(reports))
	for shardKey, report := range reports {
		status := toShardDistributorShardStatus(report.GetStatus())
		result[shardKey] = &types.ShardStatusReport{
			Status:    status,
			ShardLoad: report.GetShardLoad(),
		}
	}
	return result
}

func fromShardDistributorShardAssignments(assignments map[string]*types.ShardAssignment) map[string]*sharddistributorv1.ShardAssignment {
	if assignments == nil {
		return nil
	}

	result := make(map[string]*sharddistributorv1.ShardAssignment, len(assignments))
	for shardKey, assignment := range assignments {
		status := fromShardDistributorAssignmentStatus(assignment.GetStatus())
		result[shardKey] = &sharddistributorv1.ShardAssignment{
			Status: status,
		}
	}
	return result
}

func toShardDistributorShardAssignments(assignments map[string]*sharddistributorv1.ShardAssignment) map[string]*types.ShardAssignment {
	if assignments == nil {
		return nil
	}

	result := make(map[string]*types.ShardAssignment, len(assignments))
	for shardKey, assignment := range assignments {
		status := toShardDistributorAssignmentStatus(assignment.GetStatus())
		result[shardKey] = &types.ShardAssignment{
			Status: status,
		}
	}
	return result
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
		Executors:        executors,
		DrainedShardKeys: t.GetDrainedShardKeys(),
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
		Executors:        executors,
		DrainedShardKeys: t.GetDrainedShardKeys(),
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

	status := fromShardDistributorExecutorStatus(ex.GetStatus())
	var assigned []*sharddistributorv1.AssignedShardState
	if ex.GetAssignedShards() != nil {
		assigned = make([]*sharddistributorv1.AssignedShardState, 0, len(ex.GetAssignedShards()))
		for _, sh := range ex.GetAssignedShards() {
			assignmentStatus := fromShardDistributorAssignmentStatus(sh.GetAssignmentStatus())
			assigned = append(assigned, &sharddistributorv1.AssignedShardState{
				ShardKey:                 sh.GetShardKey(),
				AssignmentStatus:         assignmentStatus,
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

	status := toShardDistributorExecutorStatus(ex.GetStatus())
	var assigned []*types.ExecutorAssignedShardState
	if ex.GetAssignedShards() != nil {
		assigned = make([]*types.ExecutorAssignedShardState, 0, len(ex.GetAssignedShards()))
		for _, sh := range ex.GetAssignedShards() {
			assignmentStatus := toShardDistributorAssignmentStatus(sh.GetAssignmentStatus())
			assigned = append(assigned, &types.ExecutorAssignedShardState{
				ShardKey:                 sh.GetShardKey(),
				AssignmentStatus:         assignmentStatus,
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

// FromShardDistributorGetFullNamespaceStateRequest converts a types.GetFullNamespaceStateRequest to a sharddistributor GetFullNamespaceStateRequest.
func FromShardDistributorGetFullNamespaceStateRequest(t *types.GetFullNamespaceStateRequest) *sharddistributorv1.GetFullNamespaceStateRequest {
	if t == nil {
		return nil
	}
	return &sharddistributorv1.GetFullNamespaceStateRequest{
		Namespace: t.GetNamespace(),
	}
}

// ToShardDistributorGetFullNamespaceStateRequest converts a sharddistributor GetFullNamespaceStateRequest to a types.GetFullNamespaceStateRequest.
func ToShardDistributorGetFullNamespaceStateRequest(t *sharddistributorv1.GetFullNamespaceStateRequest) *types.GetFullNamespaceStateRequest {
	if t == nil {
		return nil
	}
	return &types.GetFullNamespaceStateRequest{
		Namespace: t.GetNamespace(),
	}
}

// FromShardDistributorGetFullNamespaceStateResponse converts a types.GetFullNamespaceStateResponse to a sharddistributor GetFullNamespaceStateResponse.
func FromShardDistributorGetFullNamespaceStateResponse(t *types.GetFullNamespaceStateResponse) *sharddistributorv1.GetFullNamespaceStateResponse {
	if t == nil {
		return nil
	}

	var executors map[string]*sharddistributorv1.HeartbeatState
	if t.GetExecutors() != nil {
		executors = make(map[string]*sharddistributorv1.HeartbeatState, len(t.GetExecutors()))
		for executorID, executor := range t.GetExecutors() {
			executors[executorID] = fromShardDistributorHeartbeatState(executor)
		}
	}

	var shardStats map[string]*sharddistributorv1.ShardStatistics
	if t.GetShardStats() != nil {
		shardStats = make(map[string]*sharddistributorv1.ShardStatistics, len(t.GetShardStats()))
		for shardKey, statistics := range t.GetShardStats() {
			shardStats[shardKey] = fromShardDistributorShardStatistics(statistics)
		}
	}

	var shardAssignments map[string]*sharddistributorv1.AssignedState
	if t.GetShardAssignments() != nil {
		shardAssignments = make(map[string]*sharddistributorv1.AssignedState, len(t.GetShardAssignments()))
		for executorID, assignment := range t.GetShardAssignments() {
			shardAssignments[executorID] = fromShardDistributorAssignedState(assignment)
		}
	}

	var drainedHosts map[string]*sharddistributorv1.DrainedHost
	if t.GetDrainedHosts() != nil {
		drainedHosts = make(map[string]*sharddistributorv1.DrainedHost, len(t.GetDrainedHosts()))
		for hostname, host := range t.GetDrainedHosts() {
			drainedHosts[hostname] = fromShardDistributorDrainedHost(host)
		}
	}

	return &sharddistributorv1.GetFullNamespaceStateResponse{
		Namespace:        t.GetNamespace(),
		Executors:        executors,
		ShardStats:       shardStats,
		ShardAssignments: shardAssignments,
		DrainedShards:    t.GetDrainedShards(),
		DrainedHosts:     drainedHosts,
	}
}

func fromShardDistributorHeartbeatState(t *types.HeartbeatState) *sharddistributorv1.HeartbeatState {
	if t == nil {
		return nil
	}

	lastHeartbeat := t.GetLastHeartbeat()
	status := fromShardDistributorExecutorStatus(t.GetStatus())
	reportedShards := fromShardDistributorShardStatusReports(t.GetReportedShards())
	return &sharddistributorv1.HeartbeatState{
		LastHeartbeat:  timeToTimestamp(&lastHeartbeat),
		Status:         status,
		ReportedShards: reportedShards,
		Metadata:       t.GetMetadata(),
	}
}

func fromShardDistributorShardStatistics(t *types.ShardStatistics) *sharddistributorv1.ShardStatistics {
	if t == nil {
		return nil
	}
	lastUpdateTime := t.GetLastUpdateTime()
	lastMoveTime := t.GetLastMoveTime()
	return &sharddistributorv1.ShardStatistics{
		SmoothedLoad:   t.GetSmoothedLoad(),
		LastUpdateTime: timeToTimestamp(&lastUpdateTime),
		LastMoveTime:   timeToTimestamp(&lastMoveTime),
	}
}

func fromShardDistributorAssignedState(t *types.AssignedState) *sharddistributorv1.AssignedState {
	if t == nil {
		return nil
	}

	assignedShards := fromShardDistributorShardAssignments(t.GetAssignedShards())
	var handoverStats map[string]*sharddistributorv1.ShardHandoverStats
	if t.GetShardHandoverStats() != nil {
		handoverStats = make(map[string]*sharddistributorv1.ShardHandoverStats, len(t.GetShardHandoverStats()))
		for shardKey, statistics := range t.GetShardHandoverStats() {
			var handoverType sharddistributorv1.HandoverType
			switch statistics.GetHandoverType() {
			case types.HandoverTypeINVALID:
				handoverType = sharddistributorv1.HandoverType_HANDOVER_TYPE_INVALID
			case types.HandoverTypeGRACEFUL:
				handoverType = sharddistributorv1.HandoverType_HANDOVER_TYPE_GRACEFUL
			case types.HandoverTypeEMERGENCY:
				handoverType = sharddistributorv1.HandoverType_HANDOVER_TYPE_EMERGENCY
			default:
				handoverType = sharddistributorv1.HandoverType_HANDOVER_TYPE_INVALID
			}
			previousHeartbeat := statistics.GetPreviousExecutorLastHeartbeatTime()
			handoverStats[shardKey] = &sharddistributorv1.ShardHandoverStats{
				PreviousExecutorLastHeartbeatTime: timeToTimestamp(&previousHeartbeat),
				HandoverType:                      handoverType,
			}
		}
	}

	lastUpdated := t.GetLastUpdated()
	return &sharddistributorv1.AssignedState{
		AssignedShards:     assignedShards,
		ShardHandoverStats: handoverStats,
		LastUpdated:        timeToTimestamp(&lastUpdated),
		ModRevision:        t.GetModRevision(),
	}
}

func fromShardDistributorDrainedHost(t *types.DrainedHost) *sharddistributorv1.DrainedHost {
	if t == nil {
		return nil
	}
	drainedAt := t.GetDrainedAt()
	return &sharddistributorv1.DrainedHost{
		Hostname:  t.GetHostname(),
		DrainedAt: timeToTimestamp(&drainedAt),
		DrainedBy: t.GetDrainedBy(),
		Reason:    t.GetReason(),
	}
}

// ToShardDistributorGetFullNamespaceStateResponse converts a sharddistributor GetFullNamespaceStateResponse to a types.GetFullNamespaceStateResponse.
func ToShardDistributorGetFullNamespaceStateResponse(t *sharddistributorv1.GetFullNamespaceStateResponse) *types.GetFullNamespaceStateResponse {
	if t == nil {
		return nil
	}

	var executors map[string]*types.HeartbeatState
	if t.GetExecutors() != nil {
		executors = make(map[string]*types.HeartbeatState, len(t.GetExecutors()))
		for executorID, executor := range t.GetExecutors() {
			executors[executorID] = toShardDistributorHeartbeatState(executor)
		}
	}

	var shardStats map[string]*types.ShardStatistics
	if t.GetShardStats() != nil {
		shardStats = make(map[string]*types.ShardStatistics, len(t.GetShardStats()))
		for shardKey, statistics := range t.GetShardStats() {
			shardStats[shardKey] = toShardDistributorShardStatistics(statistics)
		}
	}

	var shardAssignments map[string]*types.AssignedState
	if t.GetShardAssignments() != nil {
		shardAssignments = make(map[string]*types.AssignedState, len(t.GetShardAssignments()))
		for executorID, assignment := range t.GetShardAssignments() {
			shardAssignments[executorID] = toShardDistributorAssignedState(assignment)
		}
	}

	var drainedHosts map[string]*types.DrainedHost
	if t.GetDrainedHosts() != nil {
		drainedHosts = make(map[string]*types.DrainedHost, len(t.GetDrainedHosts()))
		for hostname, host := range t.GetDrainedHosts() {
			drainedHosts[hostname] = toShardDistributorDrainedHost(host)
		}
	}

	return &types.GetFullNamespaceStateResponse{
		Namespace:        t.GetNamespace(),
		Executors:        executors,
		ShardStats:       shardStats,
		ShardAssignments: shardAssignments,
		DrainedShards:    t.GetDrainedShards(),
		DrainedHosts:     drainedHosts,
	}
}

func toShardDistributorHeartbeatState(t *sharddistributorv1.HeartbeatState) *types.HeartbeatState {
	if t == nil {
		return nil
	}

	status := toShardDistributorExecutorStatus(t.GetStatus())
	reportedShards := toShardDistributorShardStatusReports(t.GetReportedShards())
	return &types.HeartbeatState{
		LastHeartbeat:  timestampToTimeVal(t.GetLastHeartbeat()),
		Status:         status,
		ReportedShards: reportedShards,
		Metadata:       t.GetMetadata(),
	}
}

func toShardDistributorShardStatistics(t *sharddistributorv1.ShardStatistics) *types.ShardStatistics {
	if t == nil {
		return nil
	}
	return &types.ShardStatistics{
		SmoothedLoad:   t.GetSmoothedLoad(),
		LastUpdateTime: timestampToTimeVal(t.GetLastUpdateTime()),
		LastMoveTime:   timestampToTimeVal(t.GetLastMoveTime()),
	}
}

func toShardDistributorAssignedState(t *sharddistributorv1.AssignedState) *types.AssignedState {
	if t == nil {
		return nil
	}

	assignedShards := toShardDistributorShardAssignments(t.GetAssignedShards())
	var handoverStats map[string]*types.ShardHandoverStats
	if t.GetShardHandoverStats() != nil {
		handoverStats = make(map[string]*types.ShardHandoverStats, len(t.GetShardHandoverStats()))
		for shardKey, statistics := range t.GetShardHandoverStats() {
			var handoverType types.HandoverType
			switch statistics.GetHandoverType() {
			case sharddistributorv1.HandoverType_HANDOVER_TYPE_INVALID:
				handoverType = types.HandoverTypeINVALID
			case sharddistributorv1.HandoverType_HANDOVER_TYPE_GRACEFUL:
				handoverType = types.HandoverTypeGRACEFUL
			case sharddistributorv1.HandoverType_HANDOVER_TYPE_EMERGENCY:
				handoverType = types.HandoverTypeEMERGENCY
			default:
				handoverType = types.HandoverTypeINVALID
			}
			handoverStats[shardKey] = &types.ShardHandoverStats{
				PreviousExecutorLastHeartbeatTime: timestampToTimeVal(statistics.GetPreviousExecutorLastHeartbeatTime()),
				HandoverType:                      handoverType,
			}
		}
	}

	return &types.AssignedState{
		AssignedShards:     assignedShards,
		ShardHandoverStats: handoverStats,
		LastUpdated:        timestampToTimeVal(t.GetLastUpdated()),
		ModRevision:        t.GetModRevision(),
	}
}

func toShardDistributorDrainedHost(t *sharddistributorv1.DrainedHost) *types.DrainedHost {
	if t == nil {
		return nil
	}
	return &types.DrainedHost{
		Hostname:  t.GetHostname(),
		DrainedAt: timestampToTimeVal(t.GetDrainedAt()),
		DrainedBy: t.GetDrainedBy(),
		Reason:    t.GetReason(),
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
