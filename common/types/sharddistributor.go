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

package types

import (
	"fmt"
	"time"
)

//go:generate enumer -type=ExecutorStatus,ShardStatus,AssignmentStatus,MigrationMode,HandoverType,LoadBalancingMode -json -output sharddistributor_statuses_enumer_generated.go

type GetShardOwnerRequest struct {
	ShardKey  string
	Namespace string
}

func (v *GetShardOwnerRequest) GetShardKey() (o string) {
	if v != nil {
		return v.ShardKey
	}
	return
}

func (v *GetShardOwnerRequest) GetNamespace() (o string) {
	if v != nil {
		return v.Namespace
	}
	return
}

type GetShardOwnerResponse struct {
	Owner     string
	Namespace string
	Metadata  map[string]string
}

func (v *GetShardOwnerResponse) GetOwner() (o string) {
	if v != nil {
		return v.Owner
	}
	return
}

func (v *GetShardOwnerResponse) GetNamespace() (o string) {
	if v != nil {
		return v.Namespace
	}
	return
}

func (v *GetShardOwnerResponse) GetMetadata() (o map[string]string) {
	if v != nil {
		return v.Metadata
	}
	return
}

type GetNamespaceStateRequest struct {
	Namespace string
}

func (v *GetNamespaceStateRequest) GetNamespace() (o string) {
	if v != nil {
		return v.Namespace
	}
	return
}

type GetNamespaceStateResponse struct {
	Namespace string
	Executors []*NamespaceExecutorState
}

func (v *GetNamespaceStateResponse) GetNamespace() (o string) {
	if v != nil {
		return v.Namespace
	}
	return
}

func (v *GetNamespaceStateResponse) GetExecutors() (o []*NamespaceExecutorState) {
	if v != nil {
		return v.Executors
	}
	return
}

type GetExecutorStateRequest struct {
	Namespace  string
	ExecutorID string
}

func (v *GetExecutorStateRequest) GetNamespace() (o string) {
	if v != nil {
		return v.Namespace
	}
	return
}

func (v *GetExecutorStateRequest) GetExecutorID() (o string) {
	if v != nil {
		return v.ExecutorID
	}
	return
}

type GetExecutorStateResponse struct {
	Namespace string
	Executor  *NamespaceExecutorState
}

func (v *GetExecutorStateResponse) GetNamespace() (o string) {
	if v != nil {
		return v.Namespace
	}
	return
}

func (v *GetExecutorStateResponse) GetExecutor() (o *NamespaceExecutorState) {
	if v != nil {
		return v.Executor
	}
	return
}

type NamespaceExecutorState struct {
	ExecutorID     string
	Status         ExecutorStatus
	LastHeartbeat  time.Time
	Metadata       map[string]string
	AssignedShards []*ExecutorAssignedShardState
}

func (v *NamespaceExecutorState) GetExecutorID() (o string) {
	if v != nil {
		return v.ExecutorID
	}
	return
}

func (v *NamespaceExecutorState) GetStatus() (o ExecutorStatus) {
	if v != nil {
		return v.Status
	}
	return
}

func (v *NamespaceExecutorState) GetLastHeartbeat() (o time.Time) {
	if v != nil {
		return v.LastHeartbeat
	}
	return
}

func (v *NamespaceExecutorState) GetMetadata() (o map[string]string) {
	if v != nil {
		return v.Metadata
	}
	return
}

func (v *NamespaceExecutorState) GetAssignedShards() (o []*ExecutorAssignedShardState) {
	if v != nil {
		return v.AssignedShards
	}
	return
}

type ExecutorAssignedShardState struct {
	ShardKey                 string
	AssignmentStatus         AssignmentStatus
	AssignedStateModRevision int64
}

func (v *ExecutorAssignedShardState) GetShardKey() (o string) {
	if v != nil {
		return v.ShardKey
	}
	return
}

func (v *ExecutorAssignedShardState) GetAssignmentStatus() (o AssignmentStatus) {
	if v != nil {
		return v.AssignmentStatus
	}
	return
}

func (v *ExecutorAssignedShardState) GetAssignedStateModRevision() (o int64) {
	if v != nil {
		return v.AssignedStateModRevision
	}
	return
}

type ListNamespacesRequest struct{}

// GetNamespace satisfies the metered-wrapper template which tags every scope
// with metrics.NamespaceTag. ListNamespaces is fleet-wide so this is always "".
func (v *ListNamespacesRequest) GetNamespace() string {
	return ""
}

type ListNamespacesResponse struct {
	Namespaces []*NamespaceConfig
}

func (v *ListNamespacesResponse) GetNamespaces() (o []*NamespaceConfig) {
	if v != nil {
		return v.Namespaces
	}
	return
}

// NamespaceConfig is the operator-visible static configuration of a namespace,
// loaded from the server's shardDistribution.namespaces YAML config at startup.
type NamespaceConfig struct {
	Name string
	// Type is "fixed" or "ephemeral".
	Type string
	// Mode is the migration mode (e.g. "onboarded", "local_pass", "distributed_pass").
	Mode string
	// ShardNum is the configured number of shards. Only for Type=="fixed".
	ShardNum int64
}

func (v *NamespaceConfig) GetName() (o string) {
	if v != nil {
		return v.Name
	}
	return
}

func (v *NamespaceConfig) GetType() (o string) {
	if v != nil {
		return v.Type
	}
	return
}

func (v *NamespaceConfig) GetMode() (o string) {
	if v != nil {
		return v.Mode
	}
	return
}

func (v *NamespaceConfig) GetShardNum() (o int64) {
	if v != nil {
		return v.ShardNum
	}
	return
}

type ForceResetNamespaceRequest struct {
	Namespace string
}

func (v *ForceResetNamespaceRequest) GetNamespace() (o string) {
	if v != nil {
		return v.Namespace
	}
	return
}

type ForceResetNamespaceResponse struct {
	// DeletedKeys is the number of etcd keys removed under the namespace prefix
	DeletedKeys int64
}

func (v *ForceResetNamespaceResponse) GetDeletedKeys() (o int64) {
	if v != nil {
		return v.DeletedKeys
	}
	return
}

type NamespaceNotFoundError struct {
	Namespace string
}

func (n *NamespaceNotFoundError) Error() (o string) {
	if n != nil {
		return fmt.Sprintf("namespace not found %v", n.Namespace)
	}
	return
}

type ShardNotFoundError struct {
	Namespace string
	ShardKey  string
}

func (n *ShardNotFoundError) Error() (o string) {
	if n != nil {
		return fmt.Sprintf("shard not found %v:%v", n.Namespace, n.ShardKey)
	}
	return
}

// ShardDrainedError is returned when an operation is attempted on a shard
// that is currently in the drained list for its namespace.
type ShardDrainedError struct {
	Namespace string
	ShardKey  string
}

func (n *ShardDrainedError) Error() (o string) {
	if n != nil {
		return fmt.Sprintf("shard drained %v:%v", n.Namespace, n.ShardKey)
	}
	return
}

type ExecutorHeartbeatRequest struct {
	Namespace          string
	ExecutorID         string
	Status             ExecutorStatus
	ShardStatusReports map[string]*ShardStatusReport
	Metadata           map[string]string
}

func (v *ExecutorHeartbeatRequest) GetNamespace() (o string) {
	if v != nil {
		return v.Namespace
	}
	return
}

func (v *ExecutorHeartbeatRequest) GetExecutorID() (o string) {
	if v != nil {
		return v.ExecutorID
	}
	return
}

func (v *ExecutorHeartbeatRequest) GetStatus() (o ExecutorStatus) {
	if v != nil {
		return v.Status
	}
	return
}

func (v *ExecutorHeartbeatRequest) GetShardStatusReports() (o map[string]*ShardStatusReport) {
	if v != nil {
		return v.ShardStatusReports
	}
	return
}

func (v *ExecutorHeartbeatRequest) GetMetadata() (o map[string]string) {
	if v != nil {
		return v.Metadata
	}
	return
}

// ExecutorStatus is persisted to the DB with a string value mapping.
// Beware - if we want to change the name - it should be backward compatible and should be done in two steps.
type ExecutorStatus int32

const (
	ExecutorStatusINVALID  ExecutorStatus = 0
	ExecutorStatusACTIVE   ExecutorStatus = 1
	ExecutorStatusDRAINING ExecutorStatus = 2
	ExecutorStatusDRAINED  ExecutorStatus = 3
)

type ShardStatusReport struct {
	Status    ShardStatus
	ShardLoad float64
}

func (v *ShardStatusReport) GetStatus() (o ShardStatus) {
	if v != nil {
		return v.Status
	}
	return
}

func (v *ShardStatusReport) GetShardLoad() (o float64) {
	if v != nil {
		return v.ShardLoad
	}
	return
}

// ShardStatus is persisted to the DB with a string value mapping.
// Beware - if we want to change the name - it should be backward compatible and should be done in two steps.
type ShardStatus int32

const (
	ShardStatusINVALID ShardStatus = 0
	ShardStatusREADY   ShardStatus = 1
	ShardStatusDONE    ShardStatus = 2
)

type ExecutorHeartbeatResponse struct {
	ShardAssignments map[string]*ShardAssignment
	MigrationMode    MigrationMode
}

func (v *ExecutorHeartbeatResponse) GetShardAssignments() (o map[string]*ShardAssignment) {
	if v != nil {
		return v.ShardAssignments
	}
	return
}

func (v *ExecutorHeartbeatResponse) GetMigrationMode() (o MigrationMode) {
	if v != nil {
		return v.MigrationMode
	}
	return
}

type ShardAssignment struct {
	// Status indicates the current assignment status of the shard.
	Status AssignmentStatus `json:"status"`
}

func (v *ShardAssignment) GetStatus() (o AssignmentStatus) {
	if v != nil {
		return v.Status
	}
	return
}

// AssignmentStatus is persisted to the DB with a string value mapping.
// Beware - if we want to change the name - it should be backward compatible and should be done in two steps.
type AssignmentStatus int32

const (
	AssignmentStatusINVALID AssignmentStatus = 0
	AssignmentStatusREADY   AssignmentStatus = 1
)

// HandoverType is used to indicate the type of handover that occurred during shard reassignment.
// Type is persisted to the DB with a string value mapping.
// Beware - if we want to change the name - it should be backward compatible and should be done in two steps.
type HandoverType int32

const (
	HandoverTypeINVALID HandoverType = 0

	// HandoverTypeGRACEFUL
	// Graceful handover indicates that the shard was transferred in a way that allowed
	// the previous owner had a chance to finish processing before the shard was reassigned.
	HandoverTypeGRACEFUL HandoverType = 1

	// HandoverTypeEMERGENCY
	// Emergency handover indicates that the shard was transferred abruptly without
	// allowing the previous owner to finish processing.
	HandoverTypeEMERGENCY HandoverType = 2
)

// Ptr returns a pointer to the HandoverType value.
func (t HandoverType) Ptr() *HandoverType {
	return &t
}

type MigrationMode int32

const (
	MigrationModeINVALID          MigrationMode = 0
	MigrationModeLOCALPASSTHROUGH MigrationMode = 1
	MigrationModeONBOARDED        MigrationMode = 2
)

type LoadBalancingMode int32

const (
	LoadBalancingModeINVALID LoadBalancingMode = 0
	LoadBalancingModeNAIVE   LoadBalancingMode = 1
	LoadBalancingModeGREEDY  LoadBalancingMode = 2
)

type WatchNamespaceStateRequest struct {
	Namespace string
}

func (v *WatchNamespaceStateRequest) GetNamespace() (o string) {
	if v != nil {
		return v.Namespace
	}
	return
}

type WatchNamespaceStateResponse struct {
	Executors []*ExecutorShardAssignment
}

func (v *WatchNamespaceStateResponse) GetExecutors() (o []*ExecutorShardAssignment) {
	if v != nil {
		return v.Executors
	}
	return
}

type ExecutorShardAssignment struct {
	ExecutorID     string
	AssignedShards []*Shard
	Metadata       map[string]string
}

func (v *ExecutorShardAssignment) GetExecutorID() (o string) {
	if v != nil {
		return v.ExecutorID
	}
	return
}

func (v *ExecutorShardAssignment) GetAssignedShards() (o []*Shard) {
	if v != nil {
		return v.AssignedShards
	}
	return
}

func (v *ExecutorShardAssignment) GetMetadata() (o map[string]string) {
	if v != nil {
		return v.Metadata
	}
	return
}

type Shard struct {
	ShardKey string
}

func (v *Shard) GetShardKey() (o string) {
	if v != nil {
		return v.ShardKey
	}
	return
}

type DrainShardsRequest struct {
	Namespace string
	ShardKeys []string
}

func (v *DrainShardsRequest) GetNamespace() (o string) {
	if v != nil {
		return v.Namespace
	}
	return
}

func (v *DrainShardsRequest) GetShardKeys() (o []string) {
	if v != nil {
		return v.ShardKeys
	}
	return
}

type DrainShardsResponse struct {
	// DrainedShardKeys is the set of shard keys that are drained for the
	// namespace after this call (includes shards that were already drained).
	DrainedShardKeys []string
}

func (v *DrainShardsResponse) GetDrainedShardKeys() (o []string) {
	if v != nil {
		return v.DrainedShardKeys
	}
	return
}

type UndrainShardsRequest struct {
	Namespace string
	ShardKeys []string
}

func (v *UndrainShardsRequest) GetNamespace() (o string) {
	if v != nil {
		return v.Namespace
	}
	return
}

func (v *UndrainShardsRequest) GetShardKeys() (o []string) {
	if v != nil {
		return v.ShardKeys
	}
	return
}

type UndrainShardsResponse struct {
	// UndrainedShardKeys is the subset of the request shard keys that were
	// actually removed from the drained list by this call.
	UndrainedShardKeys []string
}

func (v *UndrainShardsResponse) GetUndrainedShardKeys() (o []string) {
	if v != nil {
		return v.UndrainedShardKeys
	}
	return
}

type GetDrainedShardsRequest struct {
	Namespace string
}

func (v *GetDrainedShardsRequest) GetNamespace() (o string) {
	if v != nil {
		return v.Namespace
	}
	return
}

type GetDrainedShardsResponse struct {
	Namespace string
	ShardKeys []string
}

func (v *GetDrainedShardsResponse) GetNamespace() (o string) {
	if v != nil {
		return v.Namespace
	}
	return
}

func (v *GetDrainedShardsResponse) GetShardKeys() (o []string) {
	if v != nil {
		return v.ShardKeys
	}
	return
}
