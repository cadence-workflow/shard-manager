package process

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math/rand"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"

	"go.uber.org/fx"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/loadbalancer"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/loadbalancer/plan"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination=process_mock.go Factory,Processor

// Module provides processor factory for fx app.
var Module = fx.Module(
	"leader-process",
	fx.Provide(NewProcessorFactory),
)

// Processor represents a process that runs when the instance is the leader
type Processor interface {
	Run(ctx context.Context) error
	Terminate(ctx context.Context) error
}

// Factory creates processor instances
type Factory interface {
	// CreateProcessor creates a new processor, it takes the generic store
	// and the election object which provides the transactional guard.
	CreateProcessor(cfg config.Namespace, storage store.Store, election store.Election) Processor
}

const (
	_defaultHeartbeatTTL = 10 * time.Second
	_defaultTimeout      = 1 * time.Second
	_defaultCooldown     = 250 * time.Millisecond
)

type processorFactory struct {
	logger        log.Logger
	timeSource    clock.TimeSource
	cfg           config.LeaderProcess
	metricsClient metrics.Client
	sdConfig      *config.Config
}

type namespaceProcessor struct {
	namespaceCfg  config.Namespace
	logger        log.Logger
	metricsClient metrics.Client
	timeSource    clock.TimeSource
	running       bool
	cancel        context.CancelFunc
	sdConfig      *config.Config
	cfg           config.LeaderProcess
	wg            sync.WaitGroup
	shardStore    store.Store
	election      store.Election
}

// NewProcessorFactory creates a new processor factory
func NewProcessorFactory(
	logger log.Logger,
	metricsClient metrics.Client,
	timeSource clock.TimeSource,
	cfg config.ShardDistribution,
	sdConfig *config.Config,
) Factory {
	if cfg.Process.HeartbeatTTL <= 0 {
		cfg.Process.HeartbeatTTL = _defaultHeartbeatTTL
	}
	if cfg.Process.Timeout <= 0 {
		cfg.Process.Timeout = _defaultTimeout
	}
	if cfg.Process.RebalanceCooldown == 0 {
		cfg.Process.RebalanceCooldown = _defaultCooldown
	}

	return &processorFactory{
		logger:        logger,
		timeSource:    timeSource,
		cfg:           cfg.Process,
		metricsClient: metricsClient,
		sdConfig:      sdConfig,
	}
}

// CreateProcessor creates a new processor for the given namespace
func (f *processorFactory) CreateProcessor(cfg config.Namespace, shardStore store.Store, election store.Election) Processor {
	return &namespaceProcessor{
		namespaceCfg:  cfg,
		logger:        f.logger.WithTags(tag.ComponentLeaderProcessor, tag.ShardNamespace(cfg.Name)),
		timeSource:    f.timeSource,
		cfg:           f.cfg,
		shardStore:    shardStore,
		election:      election, // Store the election object
		metricsClient: f.metricsClient,
		sdConfig:      f.sdConfig,
	}
}

// Run begins processing for this namespace
func (p *namespaceProcessor) Run(ctx context.Context) error {
	if p.running {
		return fmt.Errorf("processor is already running")
	}

	pCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.running = true

	p.logger.Debug("Starting")

	p.wg.Go(func() { p.runTerm(pCtx) })

	return nil
}

// Terminate halts processing for this namespace
func (p *namespaceProcessor) Terminate(ctx context.Context) error {
	if !p.running {
		return fmt.Errorf("processor has not been started")
	}

	p.logger.Debug("Stopping")

	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}

	p.running = false

	// Ensure that the process has stopped.
	p.wg.Wait()

	return nil
}

// runTerm rebalances and cleans up shard stats once, then rebalances on executor changes until the term ends.
// A failed rebalance is not retried, the next term starts with a fresh one.
func (p *namespaceProcessor) runTerm(ctx context.Context) {
	// Subscribing before the initial rebalance so we don't miss any changes.
	updateChan, subscribeErr := p.shardStore.SubscribeToExecutorStatusChanges(ctx, p.namespaceCfg.Name)

	// Perform an initial rebalance on startup, even without a subscription.
	if err := p.rebalanceShards(ctx); err != nil {
		p.logRebalanceError("initial rebalance failed", "initial rebalance version conflict", err)
	}
	// Changes that arrived during the initial rebalance are queued, so they wait for the cooldown too.
	nextRebalanceAllowedAt := p.timeSource.Now().Add(p.cfg.RebalanceCooldown)

	if subscribeErr != nil {
		p.logger.Error("Failed to subscribe to state changes, stopping rebalancing loop.", tag.Error(subscribeErr))
		return
	}

	// Once per term, after the initial rebalance so it never delays it.
	p.cleanupShardStats(ctx)

	// The channel closes when the term ends or the watch fails.
	for range updateChan {
		// If an update comes in before the cooldown has expired,
		// we wait until the cooldown has passed since the last rebalance before processing it.
		// This ensures that we don't rebalance too frequently in response to a flurry of updates
		p.timeSource.Sleep(nextRebalanceAllowedAt.Sub(p.timeSource.Now()))
		nextRebalanceAllowedAt = p.timeSource.Now().Add(p.cfg.RebalanceCooldown)

		// A change left pending when the term ends would only fail against the cancelled context.
		if ctx.Err() != nil {
			return
		}

		p.logger.Info("Rebalancing triggered by state change")
		if err := p.rebalanceShards(ctx); err != nil {
			p.logRebalanceError("rebalance failed", "rebalance version conflict", err)
		}
	}
	if ctx.Err() == nil {
		p.logger.Info("Update channel closed, stopping rebalancing loop")
	}
}

// cleanupShardStats removes stale shard statistics.
func (p *namespaceProcessor) cleanupShardStats(ctx context.Context) {
	// Only perform shard stats cleanup in GREEDY load balancing mode
	if p.sdConfig.GetLoadBalancingMode(p.namespaceCfg.Name) != types.LoadBalancingModeGREEDY {
		return
	}

	namespaceState, err := p.shardStore.GetState(ctx, p.namespaceCfg.Name)
	if err != nil {
		if ctx.Err() == nil {
			p.logger.Error("Failed to get state for shard stats cleanup", tag.Error(err))
		}
		return
	}
	staleShardStats := p.identifyStaleShardStats(namespaceState)
	if len(staleShardStats) == 0 {
		return
	}
	if err := p.shardStore.DeleteShardStats(ctx, p.namespaceCfg.Name, staleShardStats, p.election.Guard()); err != nil && ctx.Err() == nil {
		p.logger.Error("Failed to delete stale shard stats", tag.Error(err))
	}
}

// identifyStaleExecutors returns a list of executors who have not reported a heartbeat recently.
func (p *namespaceProcessor) identifyStaleExecutors(namespaceState *store.NamespaceState) map[string]struct{} {
	expiredExecutors := make(map[string]struct{})
	now := p.timeSource.Now().UTC()

	for executorID, state := range namespaceState.Executors {
		if now.Sub(state.LastHeartbeat) > p.cfg.HeartbeatTTL {
			p.logger.Info("Executor has not reported a heartbeat recently", tag.ShardExecutor(executorID), tag.ShardNamespace(p.namespaceCfg.Name), tag.Value(state.LastHeartbeat))
			expiredExecutors[executorID] = struct{}{}
		}
	}

	return expiredExecutors
}

// identifyStaleShardStats returns a list of shard statistics that are no longer relevant.
func (p *namespaceProcessor) identifyStaleShardStats(namespaceState *store.NamespaceState) []string {
	activeShards := make(map[string]struct{})
	now := p.timeSource.Now().UTC()

	// 1. build set of active executors

	// add all assigned shards from executors that are ACTIVE and not stale
	for executorID, assignedState := range namespaceState.ShardAssignments {
		executor, exists := namespaceState.Executors[executorID]
		if !exists {
			continue
		}

		isActive := executor.Status == types.ExecutorStatusACTIVE
		isNotStale := now.Sub(executor.LastHeartbeat) <= p.cfg.HeartbeatTTL
		if isActive && isNotStale {
			for shardID := range assignedState.AssignedShards {
				activeShards[shardID] = struct{}{}
			}
		}
	}

	// add all shards in ReportedShards where the status is not DONE
	for _, heartbeatState := range namespaceState.Executors {
		for shardID, shardStatusReport := range heartbeatState.ReportedShards {
			if shardStatusReport.Status != types.ShardStatusDONE {
				activeShards[shardID] = struct{}{}
			}
		}
	}

	// 2. build set of stale shard stats

	// append all shard stats that are not in the active shards set
	var staleShardStats []string
	for shardID, stats := range namespaceState.ShardStats {
		if _, ok := activeShards[shardID]; ok {
			continue
		}
		recentUpdate := !stats.LastUpdateTime.IsZero() && now.Sub(stats.LastUpdateTime) <= p.cfg.HeartbeatTTL
		recentMove := !stats.LastMoveTime.IsZero() && now.Sub(stats.LastMoveTime) <= p.cfg.HeartbeatTTL
		if recentUpdate || recentMove {
			// Preserve stats that have been updated recently to allow cooldown/load history to
			// survive executor churn. These shards are likely awaiting reassignment,
			// so we don't want to delete them.
			continue
		}
		staleShardStats = append(staleShardStats, shardID)
	}

	return staleShardStats
}

// rebalanceShards is the core logic for distributing shards among active executors.
func (p *namespaceProcessor) rebalanceShards(ctx context.Context) (err error) {
	fixedShardNum := p.sdConfig.FixedShardNum(p.namespaceCfg.Name)
	namespaceCfg := config.EffectiveNamespace(p.namespaceCfg, fixedShardNum)
	metricsLoopScope := p.metricsClient.Scope(
		metrics.ShardDistributorAssignLoopScope,
		metrics.NamespaceTag(namespaceCfg.Name),
		metrics.NamespaceTypeTag(namespaceCfg.Type),
	)

	metricsLoopScope.AddCounter(metrics.ShardDistributorAssignLoopAttempts, 1)
	defer func() {
		if err != nil {
			metricsLoopScope.AddCounter(metrics.ShardDistributorAssignLoopFail, 1)
		} else {
			metricsLoopScope.AddCounter(metrics.ShardDistributorAssignLoopSuccess, 1)
		}
	}()

	start := p.timeSource.Now()
	defer func() {
		metricsLoopScope.RecordHistogramDuration(metrics.ShardDistributorAssignLoopShardRebalanceLatency, p.timeSource.Now().Sub(start))
	}()

	ctx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()

	return p.rebalanceShardsImpl(ctx, namespaceCfg, metricsLoopScope)
}

func (p *namespaceProcessor) rebalanceShardsImpl(ctx context.Context, namespaceCfg config.Namespace, metricsLoopScope metrics.Scope) (err error) {
	namespaceState, err := p.shardStore.GetState(ctx, p.namespaceCfg.Name)
	if err != nil {
		return fmt.Errorf("get state: %w", err)
	}
	p.emitActiveShardMetric(namespaceState.ShardAssignments, metricsLoopScope)
	p.emitMaxOwnersPerShardMetric(namespaceState.ShardAssignments, metricsLoopScope)
	p.emitOldestExecutorHeartbeatLag(namespaceState, metricsLoopScope)
	metricsLoopScope.UpdateGauge(metrics.ShardDistributorDrainedHosts, float64(len(namespaceState.DrainedHosts)))

	// Identify stale executors that need to be removed
	staleExecutors := p.identifyStaleExecutors(namespaceState)
	if len(staleExecutors) > 0 {
		p.logger.Info("Identified stale executors for removal", tag.ShardExecutors(slices.Collect(maps.Keys(staleExecutors))))
	}

	activeExecutors := namespaceState.AssignableExecutorIDs(staleExecutors)
	if len(activeExecutors) == 0 {
		p.logger.Debug("No active executors found. Cannot assign shards.")
		metricsLoopScope.AddCounter(metrics.ShardDistributorAssignLoopNoActiveExecutors, 1)

		// Cleanup stale executors even if no active executors remain
		if len(staleExecutors) > 0 {
			p.logger.Info("Cleaning up stale executors (no active executors)", tag.ShardExecutors(slices.Collect(maps.Keys(staleExecutors))))
			if err := p.shardStore.DeleteExecutors(ctx, p.namespaceCfg.Name, slices.Collect(maps.Keys(staleExecutors)), p.election.Guard()); err != nil {
				p.logger.Error("Failed to delete stale executors", tag.Error(err))
			}
		}
		return nil
	}
	p.logger.Info("Active executors", tag.ShardExecutors(activeExecutors))

	assignableExecutors := make(map[string]struct{}, len(activeExecutors))
	for _, executorID := range activeExecutors {
		assignableExecutors[executorID] = struct{}{}
	}

	shardsToReassign, currentAssignments := findShardsToReassign(namespaceCfg.ShardNum, assignableExecutors, namespaceState)

	executorsToUnassign := findExecutorsToUnassign(namespaceState, assignableExecutors, staleExecutors)
	if len(executorsToUnassign) > 0 {
		p.logger.Info("Emptying assignments of non-assignable executors", tag.ShardExecutors(slices.Collect(maps.Keys(executorsToUnassign))))
	}

	metricsLoopScope.AddCounter(metrics.ShardDistributorAssignLoopNumRebalancedShards, int64(len(shardsToReassign)))

	drainedAssignedShards := findDrainedAssignedShards(namespaceState, activeExecutors)
	if len(drainedAssignedShards) > 0 {
		p.logger.Info("Dropping drained shards from executors", tag.Dynamic("drained-shards", drainedAssignedShards))
	}
	metricsLoopScope.AddCounter(metrics.ShardDistributorAssignLoopDroppedDrainedShards, int64(len(drainedAssignedShards)))
	metricsLoopScope.UpdateGauge(metrics.ShardDistributorDrainedShards, float64(len(namespaceState.DrainedShards)))

	assignShardsToEmptyExecutors(currentAssignments)
	p.updateAssignments(shardsToReassign, activeExecutors, currentAssignments)

	loadBalanceMoves, err := loadbalancer.PlanRebalance(
		p.sdConfig,
		p.namespaceCfg.Name,
		namespaceState,
		currentAssignments,
		p.timeSource.Now(),
		p.logger,
		metricsLoopScope,
	)
	if err != nil {
		return fmt.Errorf("load balance: %w", err)
	}
	if err := applyMoves(currentAssignments, loadBalanceMoves); err != nil {
		return fmt.Errorf("apply load balance moves: %w", err)
	}

	p.emitExecutorMetric(namespaceState, metricsLoopScope)
	loadbalancer.EmitAssignmentImbalanceMetrics(p.sdConfig, p.namespaceCfg.Name, metricsLoopScope, currentAssignments, namespaceState)

	// Executors missing from the plan, such as draining and stale ones, lose their shards. Changes that
	// do not fit in one write wait for the next round, which plans again from what this round wrote.
	maxChangedExecutors := p.shardStore.MaxAssignShardsExecutors()
	assignmentStep := nextAssignmentStep(namespaceState.ShardAssignments, currentAssignments, assignableExecutors, staleExecutors, maxChangedExecutors)
	if assignmentStep.deferredShards > 0 {
		p.logger.Info("Assignment changes do not fit in one write, deferring the rest to the next round",
			tag.Dynamic("deferred-shards", assignmentStep.deferredShards))
	}
	metricsLoopScope.AddCounter(metrics.ShardDistributorAssignLoopDeferredShards, int64(assignmentStep.deferredShards))

	previousOwnersByShard := namespaceState.ShardOwners()
	assignmentTime := p.timeSource.Now().UTC()
	changedAssignedStates := p.buildNewAssignmentsState(namespaceState, assignmentStep.changedAssignments, previousOwnersByShard, assignmentTime)

	distributionUnchanged := len(changedAssignedStates) == 0 && len(assignmentStep.executorsToDelete) == 0
	if distributionUnchanged {
		p.logger.Info("No changes to distribution detected. Skipping rebalance.")
		return nil
	}

	p.logger.Info("Applying new shard distribution.")

	assignShardsRequest := store.AssignShardsRequest{
		ShardAssignments:  changedAssignedStates,
		ExecutorsToDelete: assignmentStep.executorsToDelete,
	}
	err = p.shardStore.AssignShards(ctx, p.namespaceCfg.Name, assignShardsRequest, p.election.Guard())
	if err != nil {
		return fmt.Errorf("assign shards: %w", err)
	}

	shardStatisticsUpdatesForAssignmentChange, prepareStatisticsErr := loadbalancer.PrepareAssignmentStatistics(
		p.sdConfig,
		p.namespaceCfg.Name,
		previousOwnersByShard,
		changedAssignedStates,
		namespaceState.ShardStats,
		assignmentTime,
	)
	if prepareStatisticsErr != nil {
		p.logger.Warn("failed to prepare shard statistics after assignment", tag.Error(prepareStatisticsErr))
	} else if len(shardStatisticsUpdatesForAssignmentChange) > 0 {
		recordStatisticsErr := p.shardStore.RecordShardStatisticsBatch(ctx, p.namespaceCfg.Name, shardStatisticsUpdatesForAssignmentChange)
		if recordStatisticsErr != nil {
			p.logger.Warn("failed to record shard statistics after assignment", tag.Error(recordStatisticsErr))
		}
	}

	return nil
}

func (p *namespaceProcessor) emitActiveShardMetric(shardAssignments map[string]store.AssignedState, metricsLoopScope metrics.Scope) {
	totalActiveShards := 0
	for _, assignedState := range shardAssignments {
		totalActiveShards += len(assignedState.AssignedShards)
	}
	metricsLoopScope.UpdateGauge(metrics.ShardDistributorActiveShards, float64(totalActiveShards))
}

func (p *namespaceProcessor) emitExecutorMetric(namespaceState *store.NamespaceState, metricsLoopScope metrics.Scope) {
	for status, count := range namespaceState.CountExecutorsByStatus() {
		metricsLoopScope.Tagged(metrics.ExecutorStatusTag(status.String())).UpdateGauge(metrics.ShardDistributorTotalExecutors, float64(count))
	}
}

func (p *namespaceProcessor) emitMaxOwnersPerShardMetric(shardAssignments map[string]store.AssignedState, metricsLoopScope metrics.Scope) {
	shardToExecutors := make(map[string][]string)
	maxOwners := 0
	errorShards := make([]string, 0)
	for executorID, assignedState := range shardAssignments {
		for shardID := range assignedState.AssignedShards {
			shardToExecutors[shardID] = append(shardToExecutors[shardID], executorID)

			count := len(shardToExecutors[shardID])
			if count > maxOwners {
				maxOwners = count
			}
			// Each multi-owned shard crosses count 2 exactly once.
			if count == 2 {
				errorShards = append(errorShards, shardID)
			}
		}
	}

	metricsLoopScope.UpdateGauge(metrics.ShardDistributorMaxExecutorsPerShard, float64(maxOwners))

	for _, shardID := range errorShards {
		executors := shardToExecutors[shardID]
		sort.Strings(executors)
		p.logger.Error("shard owned by multiple executors",
			tag.ShardKey(shardID),
			tag.ShardExecutors(executors),
			tag.ShardNamespace(p.namespaceCfg.Name))
	}
}

func (p *namespaceProcessor) emitOldestExecutorHeartbeatLag(namespaceState *store.NamespaceState, metricsLoopScope metrics.Scope) {
	if len(namespaceState.Executors) == 0 {
		return
	}

	var oldestHeartbeat time.Time
	for _, executor := range namespaceState.Executors {
		if oldestHeartbeat.IsZero() || executor.LastHeartbeat.Before(oldestHeartbeat) {
			oldestHeartbeat = executor.LastHeartbeat
		}
	}

	lag := p.timeSource.Now().Sub(oldestHeartbeat)
	metricsLoopScope.UpdateGauge(metrics.ShardDistributorOldestExecutorHeartbeatLag, float64(lag.Milliseconds()))
}

// findDrainedAssignedShards returns the unique drained shards that are still
// assigned to given executors
func findDrainedAssignedShards(namespaceState *store.NamespaceState, executors []string) []string {
	if len(namespaceState.DrainedShards) == 0 {
		return nil
	}

	drainedAssigned := make(map[string]struct{})
	for _, executorID := range executors {
		for shardID := range namespaceState.ShardAssignments[executorID].AssignedShards {
			if namespaceState.IsShardDrained(shardID) {
				drainedAssigned[shardID] = struct{}{}
			}
		}
	}
	return slices.Collect(maps.Keys(drainedAssigned))
}

func findShardsToReassign(
	shardNum int64,
	assignableExecutors map[string]struct{},
	namespaceState *store.NamespaceState,
) ([]string, map[string][]string) {
	allAvailableShards := make(map[string]struct{})
	for _, shardID := range makeShards(shardNum) {
		if namespaceState.IsShardDrained(shardID) {
			continue
		}
		allAvailableShards[shardID] = struct{}{}
	}

	shardsToReassign := make([]string, 0)
	currentAssignments := make(map[string][]string)

	for executorID := range assignableExecutors {
		currentAssignments[executorID] = []string{}
	}

	for executorID, state := range namespaceState.ShardAssignments {
		_, isAssignable := assignableExecutors[executorID]

		for shardID := range state.AssignedShards {
			if _, ok := allAvailableShards[shardID]; ok {
				delete(allAvailableShards, shardID)
				if isAssignable {
					currentAssignments[executorID] = append(currentAssignments[executorID], shardID)
				} else {
					shardsToReassign = append(shardsToReassign, shardID)
				}
			}
		}
	}

	for shardID := range allAvailableShards {
		shardsToReassign = append(shardsToReassign, shardID)
	}
	return shardsToReassign, currentAssignments
}

// findExecutorsToUnassign returns the non-assignable executors that still hold shards, for logging.
func findExecutorsToUnassign(namespaceState *store.NamespaceState, assignableExecutors map[string]struct{}, staleExecutors map[string]struct{}) map[string]struct{} {
	executorsToUnassign := make(map[string]struct{})

	for executorID, state := range namespaceState.ShardAssignments {
		// An already-emptied record has nothing left to log.
		if len(state.AssignedShards) == 0 {
			continue
		}
		if _, isAssignable := assignableExecutors[executorID]; isAssignable {
			continue
		}

		// Stale executors are logged separately.
		if _, isStale := staleExecutors[executorID]; isStale {
			continue
		}

		executorsToUnassign[executorID] = struct{}{}
	}

	return executorsToUnassign
}

func (*namespaceProcessor) updateAssignments(shardsToReassign []string, activeExecutors []string, currentAssignments map[string][]string) {
	if len(shardsToReassign) == 0 {
		return
	}

	i := rand.Intn(len(activeExecutors))
	for _, shardID := range shardsToReassign {
		executorID := activeExecutors[i%len(activeExecutors)]
		currentAssignments[executorID] = append(currentAssignments[executorID], shardID)
		i++
	}
}

func applyMoves(currentAssignments map[string][]string, moves []plan.Move) error {
	for _, move := range moves {
		idx := slices.Index(currentAssignments[move.From], move.ShardID)
		if idx == -1 {
			return fmt.Errorf("shard %s not found in source executor %s", move.ShardID, move.From)
		}

		currentAssignments[move.From][idx] = currentAssignments[move.From][len(currentAssignments[move.From])-1]
		currentAssignments[move.From] = currentAssignments[move.From][:len(currentAssignments[move.From])-1]
		currentAssignments[move.To] = append(currentAssignments[move.To], move.ShardID)
	}
	return nil
}

// buildNewAssignmentsState builds the assigned state to write for every executor in changedAssignments.
func (p *namespaceProcessor) buildNewAssignmentsState(
	namespaceState *store.NamespaceState,
	changedAssignments map[string][]string,
	previousOwnersByShard map[string]string,
	now time.Time,
) map[string]store.AssignedState {
	assignedStates := make(map[string]store.AssignedState, len(changedAssignments))
	for executorID, shards := range changedAssignments {
		assignedShardsMap := make(map[string]*types.ShardAssignment)

		for _, shardID := range shards {
			assignedShardsMap[shardID] = &types.ShardAssignment{Status: types.AssignmentStatusREADY}
		}

		assignedStates[executorID] = store.AssignedState{
			AssignedShards:     assignedShardsMap,
			LastUpdated:        now,
			ShardHandoverStats: p.buildHandoverStats(namespaceState, previousOwnersByShard, executorID, shards),
		}
	}
	return assignedStates
}

func (p *namespaceProcessor) buildHandoverStats(
	namespaceState *store.NamespaceState,
	previousOwnersByShard map[string]string,
	executorID string,
	shardIDs []string,
) map[string]store.ShardHandoverStats {
	stats := make(map[string]store.ShardHandoverStats)
	for _, shardID := range shardIDs {
		previousOwner, hasPreviousOwner := previousOwnersByShard[shardID]
		// If there is no previous owner, this is the first assignment of the shard,
		// so skip updating handover stats.
		if !hasPreviousOwner {
			continue
		}

		// If the owner is unchanged, no handover occurred.
		if previousOwner == executorID {
			continue
		}

		// Drained shards do not produce handover statistics.
		if namespaceState.IsShardDrained(shardID) {
			continue
		}
		// previous executor heartbeat is not found in namespace state
		// meaning the executor has already been cleaned up
		// skip updating handover stats
		previousExecutor, exists := namespaceState.Executor(previousOwner)
		if !exists {
			p.logger.WithTags(
				tag.ShardNamespace(p.namespaceCfg.Name),
				tag.ShardKey(shardID),
				tag.ShardExecutor(executorID),
			).Info("previous executor heartbeat not found, skipping handover stats")
			continue
		}

		handoverType := types.HandoverTypeEMERGENCY
		// Consider it a graceful handover if the previous executor was in DRAINING
		// or DRAINED status, or its host is drained.
		if previousExecutor.IsDraining() {
			handoverType = types.HandoverTypeGRACEFUL
		}
		stats[shardID] = store.ShardHandoverStats{
			HandoverType:                      handoverType,
			PreviousExecutorLastHeartbeatTime: previousExecutor.Heartbeat.LastHeartbeat,
		}
	}
	return stats
}

func (p *namespaceProcessor) logRebalanceError(failureMessage, conflictMessage string, err error) {
	if errors.Is(err, store.ErrVersionConflict) {
		p.logger.Info(conflictMessage, tag.Error(err))
		return
	}
	p.logger.Error(failureMessage, tag.Error(err))
}

func assignShardsToEmptyExecutors(currentAssignments map[string][]string) {
	emptyExecutors := make([]string, 0)
	executorsWithShards := make([]string, 0)
	minShardsCurrentlyAssigned := 0

	// Ensure the iteration is deterministic.
	executors := make([]string, 0, len(currentAssignments))
	for executorID := range currentAssignments {
		executors = append(executors, executorID)
	}
	slices.Sort(executors)

	for _, executorID := range executors {
		if len(currentAssignments[executorID]) == 0 {
			emptyExecutors = append(emptyExecutors, executorID)
		} else {
			executorsWithShards = append(executorsWithShards, executorID)
			if minShardsCurrentlyAssigned == 0 || len(currentAssignments[executorID]) < minShardsCurrentlyAssigned {
				minShardsCurrentlyAssigned = len(currentAssignments[executorID])
			}
		}
	}

	// If there are no empty executors or no executors with shards, we don't need to do anything.
	if len(emptyExecutors) == 0 || len(executorsWithShards) == 0 {
		return
	}

	// We calculate the number of shards to assign each of the empty executors. The idea is to assume all current executors have
	// the same number of shards `minShardsCurrentlyAssigned`. We use the minimum so when steeling we don't have to worry about
	// steeling more shards that the executors have.
	// We then calculate the total number of assumed shards `minShardsCurrentlyAssigned * len(executorsWithShards)` and divide it by the
	// number of current executors. This gives us the number of shards per executor, thus the number of shards to assign to each of the
	// empty executors.
	numShardsToAssignEmptyExecutors := minShardsCurrentlyAssigned * len(executorsWithShards) / len(currentAssignments)

	stealRound := 0
	for i := 0; i < numShardsToAssignEmptyExecutors; i++ {
		for _, emptyExecutor := range emptyExecutors {
			executorToSteelFrom := executorsWithShards[stealRound%len(executorsWithShards)]
			stealRound++

			stolenShard := currentAssignments[executorToSteelFrom][0]

			currentAssignments[executorToSteelFrom] = currentAssignments[executorToSteelFrom][1:]
			currentAssignments[emptyExecutor] = append(currentAssignments[emptyExecutor], stolenShard)
		}
	}
}

func makeShards(num int64) []string {
	shards := make([]string, num)
	for i := range num {
		shards[i] = strconv.FormatInt(i, 10)
	}
	return shards
}
