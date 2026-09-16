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

package handler

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/ephemeralassigner"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

func NewHandler(
	logger log.Logger,
	timeSource clock.TimeSource,
	shardDistributionCfg config.ShardDistribution,
	cfg *config.Config,
	storage store.Store,
	metricsClient metrics.Client,
) Handler {
	handler := &handlerImpl{
		logger:               logger,
		shardDistributionCfg: shardDistributionCfg,
		storage:              storage,
		timeSource:           timeSource,
		assigner:             ephemeralassigner.New(timeSource, cfg, storage, metricsClient),
	}
	handler.stopCtx, handler.cancel = context.WithCancel(context.Background())

	// prevent us from trying to serve requests before shard distributor is started and ready
	handler.startWG.Add(1)
	return handler
}

type handlerImpl struct {
	logger log.Logger

	startWG sync.WaitGroup
	stopCtx context.Context
	cancel  context.CancelFunc

	storage              store.Store
	shardDistributionCfg config.ShardDistribution
	timeSource           clock.TimeSource

	assigner *ephemeralassigner.Assigner
}

func (h *handlerImpl) Start() {
	h.assigner.Start()
	h.startWG.Done()
}

func (h *handlerImpl) Stop() {
	if h.cancel != nil {
		h.cancel()
	}
	h.assigner.Stop()
}

func (h *handlerImpl) Health(ctx context.Context) (*types.HealthStatus, error) {
	h.startWG.Wait()
	h.logger.Debug("Shard Distributor service health check endpoint reached.")
	hs := &types.HealthStatus{Ok: true, Msg: "shard distributor good"}
	return hs, nil
}

func (h *handlerImpl) GetShardOwner(ctx context.Context, request *types.GetShardOwnerRequest) (resp *types.GetShardOwnerResponse, retError error) {
	defer func() { log.CapturePanic(recover(), h.logger, &retError) }()

	namespaceIdx := slices.IndexFunc(h.shardDistributionCfg.Namespaces, func(namespace config.Namespace) bool {
		return namespace.Name == request.Namespace
	})
	if namespaceIdx == -1 {
		return nil, &types.NamespaceNotFoundError{
			Namespace: request.Namespace,
		}
	}

	shardOwner, err := h.storage.GetShardOwner(ctx, request.Namespace, request.ShardKey)

	if errors.Is(err, store.ErrShardDrained) {
		return nil, &types.ShardDrainedError{
			Namespace: request.Namespace,
			ShardKey:  request.ShardKey,
		}
	}
	if errors.Is(err, store.ErrShardNotFound) {
		if h.shardDistributionCfg.Namespaces[namespaceIdx].Type == config.NamespaceTypeEphemeral {
			return h.assigner.GetOrAssign(ctx, request)
		}

		return nil, &types.ShardNotFoundError{
			Namespace: request.Namespace,
			ShardKey:  request.ShardKey,
		}
	}
	if err != nil {
		return nil, &types.InternalServiceError{Message: fmt.Sprintf("failed to get shard owner: %v", err)}
	}

	return &types.GetShardOwnerResponse{
		Owner:     shardOwner.ExecutorID,
		Metadata:  shardOwner.Metadata,
		Namespace: request.Namespace,
	}, nil
}

// InspectShard returns the shard owner from storage
func (h *handlerImpl) InspectShard(ctx context.Context, request *types.GetShardOwnerRequest) (resp *types.GetShardOwnerResponse, retError error) {
	defer func() { log.CapturePanic(recover(), h.logger, &retError) }()

	namespaceIdx := slices.IndexFunc(h.shardDistributionCfg.Namespaces, func(namespace config.Namespace) bool {
		return namespace.Name == request.Namespace
	})
	if namespaceIdx == -1 {
		return nil, &types.NamespaceNotFoundError{
			Namespace: request.Namespace,
		}
	}

	shardOwner, err := h.storage.GetShardOwner(ctx, request.Namespace, request.ShardKey)
	if errors.Is(err, store.ErrShardDrained) {
		return nil, &types.ShardDrainedError{
			Namespace: request.Namespace,
			ShardKey:  request.ShardKey,
		}
	}
	if errors.Is(err, store.ErrShardNotFound) {
		return nil, &types.ShardNotFoundError{
			Namespace: request.Namespace,
			ShardKey:  request.ShardKey,
		}
	}
	if err != nil {
		return nil, &types.InternalServiceError{Message: fmt.Sprintf("failed to inspect shard owner: %v", err)}
	}

	return &types.GetShardOwnerResponse{
		Owner:     shardOwner.ExecutorID,
		Metadata:  shardOwner.Metadata,
		Namespace: request.Namespace,
	}, nil
}

func (h *handlerImpl) GetNamespaceState(ctx context.Context, request *types.GetNamespaceStateRequest) (resp *types.GetNamespaceStateResponse, retError error) {
	defer func() { log.CapturePanic(recover(), h.logger, &retError) }()

	h.startWG.Wait()

	namespaceIdx := slices.IndexFunc(h.shardDistributionCfg.Namespaces, func(namespace config.Namespace) bool {
		return namespace.Name == request.GetNamespace()
	})
	if namespaceIdx == -1 {
		return nil, &types.NamespaceNotFoundError{
			Namespace: request.GetNamespace(),
		}
	}

	state, err := h.storage.GetState(ctx, request.GetNamespace())
	if err != nil {
		return nil, &types.InternalServiceError{Message: fmt.Sprintf("failed to get namespace state: %v", err)}
	}

	executors := make([]*types.NamespaceExecutorState, 0, len(state.Executors))

	for executorID, heartbeat := range state.Executors {
		assignedState := state.ShardAssignments[executorID]

		assignedShards := make([]*types.ExecutorAssignedShardState, 0, len(assignedState.AssignedShards))
		for shardKey, shardAssignment := range assignedState.AssignedShards {
			status := types.AssignmentStatusINVALID
			if shardAssignment != nil {
				status = shardAssignment.Status
			}
			assignedShards = append(assignedShards, &types.ExecutorAssignedShardState{
				ShardKey:                 shardKey,
				AssignmentStatus:         status,
				AssignedStateModRevision: assignedState.ModRevision,
			})
		}

		executors = append(executors, &types.NamespaceExecutorState{
			ExecutorID:     executorID,
			Status:         heartbeat.Status,
			LastHeartbeat:  heartbeat.LastHeartbeat,
			Metadata:       heartbeat.Metadata,
			AssignedShards: assignedShards,
		})
	}

	return &types.GetNamespaceStateResponse{
		Namespace: request.GetNamespace(),
		Executors: executors,
	}, nil
}

// GetExecutorState looks up a single executor within a namespace by executor id
func (h *handlerImpl) GetExecutorState(ctx context.Context, request *types.GetExecutorStateRequest) (resp *types.GetExecutorStateResponse, retError error) {
	defer func() { log.CapturePanic(recover(), h.logger, &retError) }()

	h.startWG.Wait()

	namespaceIdx := slices.IndexFunc(h.shardDistributionCfg.Namespaces, func(namespace config.Namespace) bool {
		return namespace.Name == request.GetNamespace()
	})
	if namespaceIdx == -1 {
		return nil, &types.NamespaceNotFoundError{
			Namespace: request.GetNamespace(),
		}
	}

	executorState, err := h.storage.GetExecutorState(ctx, request.GetNamespace(), request.GetExecutorID())
	if errors.Is(err, store.ErrExecutorNotFound) {
		return nil, &types.EntityNotExistsError{
			Message: fmt.Sprintf("executor not found %v:%v", request.GetNamespace(), request.GetExecutorID()),
		}
	}
	if err != nil {
		return nil, &types.InternalServiceError{Message: fmt.Sprintf("failed to get executor state: %v", err)}
	}
	heartbeatState := executorState.Heartbeat
	assignedState := executorState.Assignment

	assignedShards := make([]*types.ExecutorAssignedShardState, 0)
	if assignedState != nil {
		assignedShards = make([]*types.ExecutorAssignedShardState, 0, len(assignedState.AssignedShards))
		for shardKey, shardAssignment := range assignedState.AssignedShards {
			status := types.AssignmentStatusINVALID
			if shardAssignment != nil {
				status = shardAssignment.Status
			}
			assignedShards = append(assignedShards, &types.ExecutorAssignedShardState{
				ShardKey:                 shardKey,
				AssignmentStatus:         status,
				AssignedStateModRevision: assignedState.ModRevision,
			})
		}
	}

	return &types.GetExecutorStateResponse{
		Namespace: request.GetNamespace(),
		Executor: &types.NamespaceExecutorState{
			ExecutorID:     request.GetExecutorID(),
			Status:         heartbeatState.Status,
			LastHeartbeat:  heartbeatState.LastHeartbeat,
			Metadata:       heartbeatState.Metadata,
			AssignedShards: assignedShards,
		},
	}, nil
}

// ForceResetNamespace deletes every key under the namespace prefix in storage.
// The namespace must be present in the static service config; the call fails
// fast with NamespaceNotFoundError otherwise
func (h *handlerImpl) ForceResetNamespace(ctx context.Context, request *types.ForceResetNamespaceRequest) (resp *types.ForceResetNamespaceResponse, retError error) {
	defer func() { log.CapturePanic(recover(), h.logger, &retError) }()

	h.startWG.Wait()

	namespace := request.GetNamespace()
	if err := h.validateNamespace(namespace); err != nil {
		return nil, err
	}

	deleted, err := h.storage.ResetNamespace(ctx, namespace)
	if err != nil {
		return nil, &types.InternalServiceError{Message: fmt.Sprintf("failed to reset namespace: %v", err)}
	}

	h.logger.Info("Force reset namespace",
		tag.ShardNamespace(namespace),
		tag.Dynamic("deleted_keys", deleted),
	)

	return &types.ForceResetNamespaceResponse{DeletedKeys: deleted}, nil
}

// ListNamespaces returns the static namespace configuration loaded from the
// server's shardDistribution.namespaces YAML at startup.
func (h *handlerImpl) ListNamespaces(_ context.Context, _ *types.ListNamespacesRequest) (resp *types.ListNamespacesResponse, retError error) {
	defer func() { log.CapturePanic(recover(), h.logger, &retError) }()

	h.startWG.Wait()

	namespaces := make([]*types.NamespaceConfig, 0, len(h.shardDistributionCfg.Namespaces))
	for _, ns := range h.shardDistributionCfg.Namespaces {
		namespaces = append(namespaces, &types.NamespaceConfig{
			Name:     ns.Name,
			Type:     ns.Type,
			Mode:     "onboarded",
			ShardNum: ns.ShardNum,
		})
	}
	return &types.ListNamespacesResponse{Namespaces: namespaces}, nil
}

func (h *handlerImpl) sendWatchResponse(namespace string, server WatchNamespaceStateServer) error {
	state, e := h.storage.GetShardAssignments(namespace)
	if e != nil {
		return &types.InternalServiceError{Message: fmt.Sprintf("failed to get shard assignments: %v", e)}
	}
	response := &types.WatchNamespaceStateResponse{
		Executors:        make([]*types.ExecutorShardAssignment, 0, len(state.ExecutorToShards)),
		DrainedShardKeys: slices.Sorted(maps.Keys(state.DrainedShards)),
	}
	for ex, shardIDs := range state.ExecutorToShards {
		response.Executors = append(response.Executors, &types.ExecutorShardAssignment{
			ExecutorID:     ex.ExecutorID,
			AssignedShards: WrapShards(shardIDs),
			Metadata:       ex.Metadata,
		})
	}

	err := server.Send(response)
	if err != nil {
		return fmt.Errorf("send response: %w", err)
	}
	return nil
}

func (h *handlerImpl) WatchNamespaceState(request *types.WatchNamespaceStateRequest, server WatchNamespaceStateServer) error {
	h.startWG.Wait()

	var stopDone <-chan struct{}
	if h.stopCtx != nil {
		stopDone = h.stopCtx.Done()
	}

	// Subscribe to state changes from storage
	notifyCh, unSubscribe, err := h.storage.SubscribeToAssignmentChanges(server.Context(), request.Namespace)
	if err != nil {
		return &types.InternalServiceError{Message: fmt.Sprintf("failed to subscribe to namespace state: %v", err)}
	}
	defer unSubscribe()

	// Send the initial state
	if err = h.sendWatchResponse(request.Namespace, server); err != nil {
		return err
	}

	// Stream subsequent updates
	for {
		select {
		case <-server.Context().Done():
			return server.Context().Err()
		case <-stopDone:
			return h.stopCtx.Err()
		case _, ok := <-notifyCh:
			if !ok {
				return fmt.Errorf("unexpected close of updates channel")
			}
			if err = h.sendWatchResponse(request.Namespace, server); err != nil {
				return err
			}
		}
	}
}

func WrapShards(shardIDs []string) []*types.Shard {
	shards := make([]*types.Shard, 0, len(shardIDs))
	for _, shardID := range shardIDs {
		shards = append(shards, &types.Shard{ShardKey: shardID})
	}
	return shards
}

// DrainShards marks the requested shards as drained for the namespace.
// A drained shard is left unassigned until it is undrained.
// The call is idempotent, so shards that are already drained stay drained
func (h *handlerImpl) DrainShards(ctx context.Context, request *types.DrainShardsRequest) (retError error) {
	defer func() { log.CapturePanic(recover(), h.logger, &retError) }()

	h.startWG.Wait()

	namespace := request.GetNamespace()
	if err := h.validateNamespace(namespace); err != nil {
		return err
	}
	shardKeys := request.GetShardKeys()
	if err := validateShardKeys(shardKeys); err != nil {
		return err
	}

	err := h.storage.DrainShards(ctx, namespace, shardKeys)
	if err != nil {
		return &types.InternalServiceError{Message: fmt.Sprintf("failed to drain shards: %v", err)}
	}

	h.logger.Info("Drained shards",
		tag.ShardNamespace(namespace),
		tag.Dynamic("requested_shards_to_drain", shardKeys),
	)

	return nil
}

// UndrainShards removes the requested shards from the namespace's drained set
// The call is idempotent, and the response returns only the shards this call removed
func (h *handlerImpl) UndrainShards(ctx context.Context, request *types.UndrainShardsRequest) (resp *types.UndrainShardsResponse, retError error) {
	defer func() { log.CapturePanic(recover(), h.logger, &retError) }()

	h.startWG.Wait()

	namespace := request.GetNamespace()
	if err := h.validateNamespace(namespace); err != nil {
		return nil, err
	}
	shardKeys := request.GetShardKeys()
	if err := validateShardKeys(shardKeys); err != nil {
		return nil, err
	}

	undrained, err := h.storage.UndrainShards(ctx, namespace, shardKeys)
	if err != nil {
		return nil, &types.InternalServiceError{Message: fmt.Sprintf("failed to undrain shards: %v", err)}
	}

	h.logger.Info("Undrained shards",
		tag.ShardNamespace(namespace),
		tag.Dynamic("requested_shards_to_undrain", shardKeys),
		tag.Dynamic("undrained_shards", undrained),
	)

	return &types.UndrainShardsResponse{UndrainedShardKeys: undrained}, nil
}

// GetDrainedShards returns the shards currently drained for the namespace.
func (h *handlerImpl) GetDrainedShards(ctx context.Context, request *types.GetDrainedShardsRequest) (resp *types.GetDrainedShardsResponse, retError error) {
	defer func() { log.CapturePanic(recover(), h.logger, &retError) }()

	h.startWG.Wait()

	namespace := request.GetNamespace()
	if err := h.validateNamespace(namespace); err != nil {
		return nil, err
	}

	shardKeys, err := h.storage.GetDrainedShards(ctx, namespace)
	if err != nil {
		return nil, &types.InternalServiceError{Message: fmt.Sprintf("failed to get drained shards: %v", err)}
	}

	return &types.GetDrainedShardsResponse{
		Namespace: namespace,
		ShardKeys: shardKeys,
	}, nil
}

const maxHostnameLength = 128

// DrainHosts marks the requested hosts as drained for the namespace.
// Executors on a drained host are ineligible for assignment until undrained.
// The call is idempotent, so hosts that are already drained keep their original metadata.
func (h *handlerImpl) DrainHosts(ctx context.Context, request *types.DrainHostsRequest) (retError error) {
	defer func() { log.CapturePanic(recover(), h.logger, &retError) }()

	h.startWG.Wait()

	namespace := request.GetNamespace()
	if err := h.validateNamespace(namespace); err != nil {
		return err
	}
	hosts, err := toStoreDrainedHosts(request.GetHosts())
	if err != nil {
		return err
	}

	err = h.storage.DrainHosts(ctx, namespace, hosts)
	if err != nil {
		return &types.InternalServiceError{Message: fmt.Sprintf("failed to drain hosts: %v", err)}
	}

	hostnames := make([]string, 0, len(hosts))
	for _, host := range hosts {
		hostnames = append(hostnames, host.Hostname)
	}
	h.logger.Info("Drained hosts",
		tag.ShardNamespace(namespace),
		tag.Dynamic("requested_hosts_to_drain", hostnames),
	)

	return nil
}

// UndrainHosts removes the requested hosts from the namespace's drained set.
// The call is idempotent, and the response returns only the hosts this call removed.
func (h *handlerImpl) UndrainHosts(ctx context.Context, request *types.UndrainHostsRequest) (resp *types.UndrainHostsResponse, retError error) {
	defer func() { log.CapturePanic(recover(), h.logger, &retError) }()

	h.startWG.Wait()

	namespace := request.GetNamespace()
	if err := h.validateNamespace(namespace); err != nil {
		return nil, err
	}
	hostnames := request.GetHostnames()
	if err := validateHostnames(hostnames); err != nil {
		return nil, err
	}

	undrained, err := h.storage.UndrainHosts(ctx, namespace, hostnames)
	if err != nil {
		return nil, &types.InternalServiceError{Message: fmt.Sprintf("failed to undrain hosts: %v", err)}
	}

	h.logger.Info("Undrained hosts",
		tag.ShardNamespace(namespace),
		tag.Dynamic("requested_hosts_to_undrain", hostnames),
		tag.Dynamic("undrained_hosts", undrained),
	)

	return &types.UndrainHostsResponse{UndrainedHostnames: undrained}, nil
}

// GetDrainedHosts returns the hosts currently drained for the namespace.
func (h *handlerImpl) GetDrainedHosts(ctx context.Context, request *types.GetDrainedHostsRequest) (resp *types.GetDrainedHostsResponse, retError error) {
	defer func() { log.CapturePanic(recover(), h.logger, &retError) }()

	h.startWG.Wait()

	namespace := request.GetNamespace()
	if err := h.validateNamespace(namespace); err != nil {
		return nil, err
	}

	hosts, err := h.storage.GetDrainedHosts(ctx, namespace)
	if err != nil {
		return nil, &types.InternalServiceError{Message: fmt.Sprintf("failed to get drained hosts: %v", err)}
	}

	return &types.GetDrainedHostsResponse{
		Namespace: namespace,
		Hosts:     fromStoreDrainedHosts(hosts),
	}, nil
}

// validateNamespace rejects namespaces that are absent from the static service config
func (h *handlerImpl) validateNamespace(namespace string) error {
	found := slices.ContainsFunc(h.shardDistributionCfg.Namespaces, func(n config.Namespace) bool {
		return n.Name == namespace
	})
	if !found {
		return &types.NamespaceNotFoundError{Namespace: namespace}
	}
	return nil
}

// validateShardKeys rejects drain and undrain requests that storage cannot represent
func validateShardKeys(shardKeys []string) error {
	if len(shardKeys) == 0 {
		return &types.BadRequestError{Message: "shard keys must not be empty"}
	}
	for _, shardKey := range shardKeys {
		if shardKey == "" || strings.Contains(shardKey, "/") {
			return &types.BadRequestError{
				Message: fmt.Sprintf("invalid shard key %q: must be non-empty and must not contain '/'", shardKey),
			}
		}
	}
	return nil
}

func toStoreDrainedHosts(hosts []*types.DrainedHost) ([]store.DrainedHost, error) {
	if len(hosts) == 0 {
		return nil, &types.BadRequestError{Message: "hosts must not be empty"}
	}
	out := make([]store.DrainedHost, 0, len(hosts))
	hostnames := make([]string, 0, len(hosts))
	for _, host := range hosts {
		if host == nil {
			return nil, &types.BadRequestError{Message: "hosts must not contain a nil entry"}
		}
		hostnames = append(hostnames, host.GetHostname())
		out = append(out, store.DrainedHost{
			Hostname:  host.GetHostname(),
			DrainedAt: host.GetDrainedAt(),
			DrainedBy: host.GetDrainedBy(),
			Reason:    host.GetReason(),
		})
	}
	if err := validateHostnames(hostnames); err != nil {
		return nil, err
	}
	return out, nil
}

func fromStoreDrainedHosts(hosts []store.DrainedHost) []*types.DrainedHost {
	if hosts == nil {
		return nil
	}
	out := make([]*types.DrainedHost, 0, len(hosts))
	for _, host := range hosts {
		out = append(out, &types.DrainedHost{
			Hostname:  host.Hostname,
			DrainedAt: host.DrainedAt,
			DrainedBy: host.DrainedBy,
			Reason:    host.Reason,
		})
	}
	return out
}

// validateHostnames rejects drain and undrain requests that storage cannot represent.
func validateHostnames(hostnames []string) error {
	if len(hostnames) == 0 {
		return &types.BadRequestError{Message: "hostnames must not be empty"}
	}
	for _, hostname := range hostnames {
		if hostname == "" || strings.Contains(hostname, "/") || strings.Contains(hostname, "@") {
			return &types.BadRequestError{
				Message: fmt.Sprintf("invalid hostname %q: must be non-empty and must not contain '/' or '@'", hostname),
			}
		}
		if len(hostname) > maxHostnameLength {
			return &types.BadRequestError{
				Message: fmt.Sprintf("invalid hostname %q: exceeds %d bytes", hostname, maxHostnameLength),
			}
		}
	}
	return nil
}
