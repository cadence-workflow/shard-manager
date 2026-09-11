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

// Package ephemeralassigner assigns ephemeral shards to executors on demand.
// Cache-miss GetShardOwner calls for ephemeral namespaces are coalesced: the
// first request starts a short collection window, and requests that arrive while
// a flush is in-flight are batched into the next flush. Each flush reads state
// once and persists all new assignments in one write.
package ephemeralassigner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cadence-workflow/shard-manager/common/backoff"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/loadbalancer"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/loadbalancer/plan"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

const (
	// ephemeralBatchTimeout is the context timeout for each coalesced batch flush.
	// Sized as a safety net rather than a target. It covers conflict retries and a
	// cold-cache GetExecutor, whose namespace refresh is itself bounded by
	// refreshOperationTimeout (5s).
	ephemeralBatchTimeout          = 5 * time.Second
	ephemeralBatchCoalescingWindow = 10 * time.Millisecond

	// versionConflictRetryInitialInterval is the starting backoff for retries
	// triggered when a concurrent shard assignment causes a version conflict.
	versionConflictRetryInitialInterval = 50 * time.Millisecond
	// versionConflictRetryMaxInterval caps the per-attempt sleep.
	versionConflictRetryMaxInterval = 1 * time.Second
	// versionConflictRetryMaxAttempts is the maximum number of retry attempts
	// before the error is surfaced to the caller.
	versionConflictRetryMaxAttempts = 3
)

// Assigner assigns ephemeral shards that do not yet exist in storage. It owns a
// shardBatcher that collects concurrent requests into batches and plans/persists
// each batch with a single pair of storage operations.
type Assigner struct {
	timeSource clock.TimeSource
	cfg        *config.Config
	storage    store.Store
	metrics    metrics.Scope

	batcher *shardBatcher
}

// New builds an Assigner. Call Start before serving requests and Stop on shutdown.
func New(timeSource clock.TimeSource, cfg *config.Config, storage store.Store, metricsClient metrics.Client) *Assigner {
	a := &Assigner{
		timeSource: timeSource,
		cfg:        cfg,
		storage:    storage,
		metrics:    metricsClient.Scope(metrics.ShardDistributorEphemeralAssignmentScope),
	}
	a.batcher = newShardBatcher(timeSource, ephemeralBatchTimeout, ephemeralBatchCoalescingWindow, a.assignEphemeralBatch)
	return a
}

// Start launches the background batching loop.
func (a *Assigner) Start() {
	a.batcher.Start()
}

// Stop signals the batching loop to shut down and waits for it to finish.
func (a *Assigner) Stop() {
	a.batcher.Stop()
}

// GetOrAssign assigns an ephemeral shard that does not yet exist in storage.
func (a *Assigner) GetOrAssign(ctx context.Context, request *types.GetShardOwnerRequest) (*types.GetShardOwnerResponse, error) {
	resp, err := a.batcher.Submit(ctx, request)
	if err != nil {
		return nil, mapAssignError(request, err)
	}
	return resp, nil
}

// mapAssignError maps assignment failures to API errors
func mapAssignError(request *types.GetShardOwnerRequest, err error) error {
	var drained *types.ShardDrainedError
	if errors.As(err, &drained) {
		return err
	}
	if errors.Is(err, store.ErrShardDrained) {
		return &types.ShardDrainedError{
			Namespace: request.Namespace,
			ShardKey:  request.ShardKey,
		}
	}
	return &types.InternalServiceError{Message: fmt.Sprintf("failed to assign ephemeral shard: %v", err)}
}

// assignEphemeralBatch is the ephemeralAssignmentBatchFn wired into the shardBatcher.
// It processes a whole batch of unassigned shard keys for a single ephemeral
// namespace using up to two storage operations per attempt:
//  1. GetState — read current namespace state once for the whole batch.
//  2. AssignShards — write all new assignments atomically in one operation.
//
// Shards that already have an owner are skipped from placement,
// the rest are placed by the load balancer and saved in a single AssignShards call
func (a *Assigner) assignEphemeralBatch(ctx context.Context, namespace string, shardKeys []string) (map[string]*types.GetShardOwnerResponse, map[string]struct{}, error) {
	batchMetrics := a.metrics.Tagged(metrics.NamespaceTag(namespace))
	batchMetrics.RecordHistogramValue(metrics.ShardDistributorEphemeralAssignmentBatchSize, float64(len(shardKeys)))

	retryPolicy := backoff.NewExponentialRetryPolicy(versionConflictRetryInitialInterval)
	retryPolicy.SetMaximumInterval(versionConflictRetryMaxInterval)
	retryPolicy.SetMaximumAttempts(versionConflictRetryMaxAttempts)

	throttleRetry := backoff.NewThrottleRetry(
		backoff.WithRetryPolicy(retryPolicy),
		backoff.WithRetryableError(func(err error) bool {
			return errors.Is(err, store.ErrVersionConflict)
		}),
		backoff.WithClock(a.timeSource),
	)

	var results map[string]*types.GetShardOwnerResponse
	var drained map[string]struct{}
	err := throttleRetry.Do(ctx, func(ctx context.Context) error {
		var err error
		results, drained, err = a.tryAssignEphemeralBatch(ctx, namespace, shardKeys, batchMetrics)
		return err
	})
	return results, drained, err
}

func (a *Assigner) tryAssignEphemeralBatch(
	ctx context.Context,
	namespace string,
	shardKeys []string,
	batchMetrics metrics.Scope,
) (map[string]*types.GetShardOwnerResponse, map[string]struct{}, error) {
	state, err := a.storage.GetState(ctx, namespace)
	if err != nil {
		return nil, nil, &types.InternalServiceError{Message: fmt.Sprintf("get namespace state: %v", err)}
	}

	executorByShard, toPlace, drained := resolveOwners(state, shardKeys)

	if len(toPlace) > 0 {
		placements, err := loadbalancer.PlanInitialPlacement(a.cfg, namespace, state, toPlace)
		if err != nil {
			return nil, nil, &types.InternalServiceError{Message: fmt.Sprintf("plan initial placement: %v", err)}
		}

		changedExecutors := mergePlacements(state, placements, a.timeSource.Now().UTC())

		writeErr := a.storage.AssignShards(ctx, namespace, store.AssignShardsRequest{
			NewState:         state,
			ChangedExecutors: changedExecutors,
		}, store.NopGuard())
		recordAssignmentWriteAttempt(batchMetrics, writeErr)

		if writeErr != nil {
			if errors.Is(writeErr, store.ErrVersionConflict) {
				// Return the version-conflict sentinel wrapped so the batch retry can
				// detect it with errors.Is.
				return nil, nil, fmt.Errorf("assign ephemeral shards: %w", writeErr)
			}
			return nil, nil, &types.InternalServiceError{Message: fmt.Sprintf("assign ephemeral shards: %v", writeErr)}
		}

		for _, placement := range placements {
			executorByShard[placement.ShardID] = placement.ExecutorID
		}
	}

	executorOwners, err := a.fetchExecutorMetadata(ctx, namespace, executorByShard)
	if err != nil {
		return nil, nil, err
	}

	return buildResults(namespace, shardKeys, executorByShard, executorOwners), drained, nil
}

func recordAssignmentWriteAttempt(scope metrics.Scope, err error) {
	writeResult := metrics.ShardDistributorAssignmentWriteResultSuccess
	if err != nil {
		writeResult = metrics.ShardDistributorAssignmentWriteResultError
		if errors.Is(err, store.ErrVersionConflict) {
			writeResult = metrics.ShardDistributorAssignmentWriteResultVersionConflict
		}
	}
	scope.Tagged(
		metrics.ShardDistributorAssignmentWriteResultTag(writeResult),
	).IncCounter(metrics.ShardDistributorEphemeralAssignmentWriteAttempts)
}

// resolveOwners splits the requested shards into those already assigned to an
// executor, mapped to that current owner, and those still needing placement
func resolveOwners(state *store.NamespaceState, shardKeys []string) (executorByShard map[string]string, toPlace []string, drained map[string]struct{}) {
	owners := state.ShardOwners()
	executorByShard = make(map[string]string, len(shardKeys))
	for _, shardKey := range shardKeys {
		if _, isDrained := state.DrainedShards[shardKey]; isDrained {
			continue
		}
		if executorID, ok := owners[shardKey]; ok {
			executorByShard[shardKey] = executorID
			continue
		}
		toPlace = append(toPlace, shardKey)
	}
	return executorByShard, toPlace, state.DrainedShards
}

// mergePlacements folds the planned shard→executor placements back into state
// and returns the set of modified executors.
// The AssignedShards maps are copied to avoid mutating the object returned by
// GetState.
func mergePlacements(state *store.NamespaceState, placements []plan.Placement, now time.Time) map[string]struct{} {
	// Track executors whose assignments change so persistence can avoid rewriting unchanged executor state.
	changedExecutors := make(map[string]struct{})
	if state.ShardAssignments == nil {
		state.ShardAssignments = make(map[string]store.AssignedState)
	}
	for executorID, shardsForExecutor := range placementsByExecutor(placements) {
		existing := state.ShardAssignments[executorID]
		newShards := make(map[string]*types.ShardAssignment, len(existing.AssignedShards)+len(shardsForExecutor))
		for k, v := range existing.AssignedShards {
			newShards[k] = v
		}
		for _, shardKey := range shardsForExecutor {
			newShards[shardKey] = &types.ShardAssignment{Status: types.AssignmentStatusREADY}
		}
		existing.AssignedShards = newShards
		existing.LastUpdated = now
		state.ShardAssignments[executorID] = existing
		changedExecutors[executorID] = struct{}{}
	}
	return changedExecutors
}

// fetchExecutorMetadata calls GetExecutor once per unique executor referenced by
// the resolved owners. Metadata is stored separately from HeartbeatState and is
// not returned by GetState.
func (a *Assigner) fetchExecutorMetadata(ctx context.Context, namespace string, executorByShard map[string]string) (map[string]*store.ShardOwner, error) {
	executorOwners := make(map[string]*store.ShardOwner, len(executorByShard))
	for _, executorID := range executorByShard {
		if _, already := executorOwners[executorID]; already {
			continue
		}
		owner, err := a.storage.GetExecutor(ctx, namespace, executorID)
		if err != nil {
			return nil, &types.InternalServiceError{Message: fmt.Sprintf("get executor %q: %v", executorID, err)}
		}
		executorOwners[executorID] = owner
	}
	return executorOwners, nil
}

// buildResults constructs the shardKey -> GetShardOwnerResponse map from the
// resolved owners and their fetched metadata.
// Shards in the drained set are omitted here; the batcher maps them to
// ShardDrainedError from the drained set returned alongside these results.
func buildResults(namespace string, shardKeys []string, executorByShard map[string]string, executorOwners map[string]*store.ShardOwner) map[string]*types.GetShardOwnerResponse {
	results := make(map[string]*types.GetShardOwnerResponse, len(shardKeys))
	for _, shardKey := range shardKeys {
		executorID, ok := executorByShard[shardKey]
		if !ok {
			// shard has no owner: a drained shard skipped by resolveOwners
			continue
		}
		owner := executorOwners[executorID]
		results[shardKey] = &types.GetShardOwnerResponse{
			Owner:     owner.ExecutorID,
			Namespace: namespace,
			Metadata:  owner.Metadata,
		}
	}
	return results
}

// placementsByExecutor turns planned placements into map[executorID][]shardKey.
func placementsByExecutor(placements []plan.Placement) map[string][]string {
	out := make(map[string][]string)
	for _, placement := range placements {
		out[placement.ExecutorID] = append(out[placement.ExecutorID], placement.ShardID)
	}
	return out
}
