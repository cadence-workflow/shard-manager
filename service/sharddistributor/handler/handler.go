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
	"slices"
	"sync"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
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
) Handler {
	handler := &handlerImpl{
		logger:               logger,
		shardDistributionCfg: shardDistributionCfg,
		storage:              storage,
		timeSource:           timeSource,
		assigner:             ephemeralassigner.New(timeSource, cfg, storage),
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

	heartbeatState, assignedState, err := h.storage.GetHeartbeat(ctx, request.GetNamespace(), request.GetExecutorID())
	if errors.Is(err, store.ErrExecutorNotFound) {
		return nil, &types.EntityNotExistsError{
			Message: fmt.Sprintf("executor not found %v:%v", request.GetNamespace(), request.GetExecutorID()),
		}
	}
	if err != nil {
		return nil, &types.InternalServiceError{Message: fmt.Sprintf("failed to get executor state: %v", err)}
	}

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
	namespaceIdx := slices.IndexFunc(h.shardDistributionCfg.Namespaces, func(n config.Namespace) bool {
		return n.Name == namespace
	})
	if namespaceIdx == -1 {
		return nil, &types.NamespaceNotFoundError{Namespace: namespace}
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

func (h *handlerImpl) WatchNamespaceState(request *types.WatchNamespaceStateRequest, server WatchNamespaceStateServer) error {
	h.startWG.Wait()

	var stopDone <-chan struct{}
	subscribeCtx := server.Context()
	if h.stopCtx != nil {
		stopDone = h.stopCtx.Done()
		subscribeCtx = h.stopCtx
	}

	// Subscribe to state changes from storage
	assignmentChangesChan, unSubscribe, err := h.storage.SubscribeToAssignmentChanges(subscribeCtx, request.Namespace)
	defer unSubscribe()
	if err != nil {
		return &types.InternalServiceError{Message: fmt.Sprintf("failed to subscribe to namespace state: %v", err)}
	}

	// Stream subsequent updates
	for {
		select {
		case <-server.Context().Done():
			return server.Context().Err()
		case <-stopDone:
			return h.stopCtx.Err()
		case assignmentChanges, ok := <-assignmentChangesChan:
			if !ok {
				return fmt.Errorf("unexpected close of updates channel")
			}
			response := &types.WatchNamespaceStateResponse{
				Executors: make([]*types.ExecutorShardAssignment, 0, len(assignmentChanges)),
			}
			for executor, shardIDs := range assignmentChanges {
				response.Executors = append(response.Executors, &types.ExecutorShardAssignment{
					ExecutorID:     executor.ExecutorID,
					AssignedShards: WrapShards(shardIDs),
					Metadata:       executor.Metadata,
				})
			}

			err = server.Send(response)
			if err != nil {
				return fmt.Errorf("send response: %w", err)
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

func (h *handlerImpl) DrainShards(ctx context.Context, request *types.DrainShardsRequest) (resp *types.DrainShardsResponse, retError error) {
	defer func() { log.CapturePanic(recover(), h.logger, &retError) }()
	return nil, &types.InternalServiceError{Message: "DrainShards is not yet implemented"}
}

func (h *handlerImpl) UndrainShards(ctx context.Context, request *types.UndrainShardsRequest) (resp *types.UndrainShardsResponse, retError error) {
	defer func() { log.CapturePanic(recover(), h.logger, &retError) }()
	return nil, &types.InternalServiceError{Message: "UndrainShards is not yet implemented"}
}

func (h *handlerImpl) GetDrainedShards(ctx context.Context, request *types.GetDrainedShardsRequest) (resp *types.GetDrainedShardsResponse, retError error) {
	defer func() { log.CapturePanic(recover(), h.logger, &retError) }()
	return nil, &types.InternalServiceError{Message: "GetDrainedShards is not yet implemented"}
}
