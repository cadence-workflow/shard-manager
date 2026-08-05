package executorclient

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uber-go/tally"
	"go.uber.org/zap"

	"github.com/cadence-workflow/shard-manager/common/backoff"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/clientcommon"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/clientcommon/tag"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient/heartbeat"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient/metricsconstants"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient/syncgeneric"
)

type processorState int32

const (
	processorStateStarting processorState = iota
	processorStateStarted
	processorStateStopping
)

const (
	heartbeatJitterCoeff           = 0.1              // 10% jitter
	processorAsyncOperationTimeout = 10 * time.Second //  maximum time allowed for a shard processor Start or Stop call.
)

type managedProcessor[SP ShardProcessor] struct {
	processor SP
	state     atomic.Int32
}

type syncExecutorMetadata struct {
	sync.RWMutex

	data map[string]string
}

func (m *syncExecutorMetadata) Set(metadata map[string]string) {
	m.Lock()
	defer m.Unlock()

	m.data = metadata
}

func (m *syncExecutorMetadata) Get() map[string]string {
	m.RLock()
	defer m.RUnlock()

	// Copy the map
	result := make(map[string]string, len(m.data))
	for k, v := range m.data {
		result[k] = v
	}

	return result
}

func (mp *managedProcessor[SP]) setState(state processorState) {
	mp.state.Store(int32(state))
}

func (mp *managedProcessor[SP]) getState() processorState {
	return processorState(mp.state.Load())
}

func newManagedProcessor[SP ShardProcessor](processor SP, state processorState) *managedProcessor[SP] {
	managed := &managedProcessor[SP]{
		processor: processor,
		state:     atomic.Int32{},
	}

	managed.setState(state)
	return managed
}

type executorImpl[SP ShardProcessor] struct {
	logger                *zap.Logger
	shardProcessorFactory ShardProcessorFactory[SP]
	namespace             string
	executorID            string
	stopC                 chan struct{}
	heartBeatInterval     time.Duration
	ttlShard              time.Duration
	managedProcessors     syncgeneric.Map[string, *managedProcessor[SP]]
	processorsToLastUse   syncgeneric.Map[string, time.Time]
	timeSource            clock.TimeSource
	processLoopWG         sync.WaitGroup
	assignmentMutex       sync.Mutex
	heartbeater           *heartbeat.Manager
	metrics               tally.Scope
	enabled               func() bool
	metadata              syncExecutorMetadata
	drainObserver         clientcommon.DrainSignalObserver
}

func (e *executorImpl[SP]) Start(ctx context.Context) {
	e.logger.Info("starting shard distributor executor", zap.String(tag.Namespace, e.namespace))
	e.processLoopWG.Add(2)
	go func() {
		defer e.processLoopWG.Done()
		e.heartbeatloop(context.WithoutCancel(ctx))
	}()
	go func() {
		defer e.processLoopWG.Done()
		e.shardCleanUpLoop(context.WithoutCancel(ctx))
	}()
}

func (e *executorImpl[SP]) Stop() {
	e.logger.Info("stopping shard distributor executor", zap.String(tag.Namespace, e.namespace))
	close(e.stopC)
	e.processLoopWG.Wait()
}

func (e *executorImpl[SP]) GetShardProcess(ctx context.Context, shardID string) (SP, error) {
	e.processorsToLastUse.Store(shardID, e.timeSource.Now())

	shardProcess, ok := e.managedProcessors.Load(shardID)
	if !ok {
		if !e.enabled() {
			// Executor is not enabled (e.g. not yet onboarded): treat as not
			// owned so callers fall back to their local ownership logic.
			var zero SP
			return zero, fmt.Errorf("%w for shard ID: %s", ErrShardProcessNotFound, shardID)
		}

		// Do a heartbeat and check again
		err := e.heartbeatAndUpdateAssignment(ctx, shardID)
		if err != nil {
			var zero SP
			return zero, fmt.Errorf("heartbeat and assign shards: %w", err)
		}

		// Check again if the shard process is found
		shardProcess, ok = e.managedProcessors.Load(shardID)
		if !ok {
			var zero SP
			return zero, fmt.Errorf("%w for shard ID: %s", ErrShardProcessNotFound, shardID)
		}
	}

	return shardProcess.processor, nil
}

// drainChannel returns the drain signal channel, or nil if no observer is configured.
func (e *executorImpl[SP]) drainChannel() <-chan struct{} {
	if e.drainObserver != nil {
		return e.drainObserver.Drain()
	}
	return nil
}

func (e *executorImpl[SP]) heartbeatloop(ctx context.Context) {
	heartBeatTimer := e.timeSource.NewTimer(backoff.JitDuration(e.heartBeatInterval, heartbeatJitterCoeff))
	defer heartBeatTimer.Stop()

	drainCh := e.drainChannel()

	for {
		select {
		case <-ctx.Done():
			e.logger.Info("shard distributor executor context done, stopping")
			e.stopShardProcessors()
			e.sendDrainingHeartbeat()
			return
		case <-e.stopC:
			e.logger.Info("shard distributor executor stopped")
			e.stopShardProcessors()
			e.sendDrainingHeartbeat()
			return
		case <-drainCh:
			e.logger.Info("drain signal received, stopping shard processors")
			e.stopShardProcessors()
			e.sendDrainingHeartbeat()

			if !e.waitForUndrain(ctx) {
				return
			}

			e.logger.Info("undrain signal received, resuming heartbeat")
			drainCh = e.drainObserver.Drain()
			heartBeatTimer.Reset(backoff.JitDuration(e.heartBeatInterval, heartbeatJitterCoeff))
		case <-heartBeatTimer.Chan():
			heartBeatTimer.Reset(backoff.JitDuration(e.heartBeatInterval, heartbeatJitterCoeff))
			// Skip heartbeating while disabled (e.g. not yet onboarded). The
			// guard is re-checked every tick so the executor starts heartbeating
			// as soon as it becomes enabled.
			if !e.enabled() {
				continue
			}
			err := e.heartbeatAndUpdateAssignment(ctx, "")
			if err != nil {
				e.logger.Error("failed to heartbeat and assign shards", zap.Error(err))
				continue
			}
		}
	}
}

// waitForUndrain blocks until the undrain signal fires or the executor is stopped.
// Returns true if undrained (caller should resume), false if stopped.
func (e *executorImpl[SP]) waitForUndrain(ctx context.Context) bool {
	if e.drainObserver == nil {
		return false
	}

	undrainCh := e.drainObserver.Undrain()

	select {
	case <-ctx.Done():
		return false
	case <-e.stopC:
		return false
	case <-undrainCh:
		return true
	}
}

func (e *executorImpl[SP]) heartbeatAndUpdateAssignment(ctx context.Context, shardID string) error {
	assignments, err := e.heartbeater.Heartbeat(ctx, shardID)
	if err != nil {
		return err
	}
	if assignments != nil {
		e.updateShardAssignmentMetered(ctx, assignments)
	}
	return nil
}

func (e *executorImpl[SP]) updateShardAssignmentMetered(ctx context.Context, shardAssignment map[string]*types.ShardAssignment) {
	e.assignmentMutex.Lock()
	defer e.assignmentMutex.Unlock()

	startTime := e.timeSource.Now()
	defer e.metrics.
		Histogram(metricsconstants.ShardDistributorExecutorAssignLoopLatency, metricsconstants.ShardDistributorExecutorAssignLoopLatencyBuckets).
		RecordDuration(e.timeSource.Since(startTime))

	e.updateShardAssignment(ctx, shardAssignment)
}

func (e *executorImpl[SP]) GetShardStatusReports() map[string]*types.ShardStatusReport {
	reports := make(map[string]*types.ShardStatusReport)
	e.managedProcessors.Range(func(shardID string, managedProcessor *managedProcessor[SP]) bool {
		if managedProcessor.getState() == processorStateStarted {
			shardStatus := managedProcessor.processor.GetShardReport()
			reports[shardID] = &types.ShardStatusReport{
				ShardLoad: shardStatus.ShardLoad,
				Status:    shardStatus.Status,
			}
		}
		return true
	})
	return reports
}

func (e *executorImpl[SP]) sendDrainingHeartbeat() {
	if !e.enabled() {
		return
	}
	if err := e.heartbeater.DrainingHeartbeat(); err != nil {
		e.logger.Error("failed to send draining heartbeat", zap.Error(err))
	}
}

func (e *executorImpl[SP]) updateShardAssignment(ctx context.Context, shardAssignments map[string]*types.ShardAssignment) {
	// Stop shards no longer assigned. Each call fires 2 goroutines: one for the
	// Stop() call and one per-shard timeout watcher.
	e.managedProcessors.Range(func(shardID string, managedProcessor *managedProcessor[SP]) bool {
		if assignment, ok := shardAssignments[shardID]; !ok || assignment.Status != types.AssignmentStatusREADY {
			e.stopManagerProcessor(shardID)
		}
		return true
	})

	// Start newly assigned shards. Each call fires 2 goroutines: one for the
	// Start() call and one per-shard timeout watcher.
	for shardID, assignment := range shardAssignments {
		if assignment.Status == types.AssignmentStatusREADY {
			e.addManagerProcessor(ctx, shardID)
		}
	}
}

func (e *executorImpl[SP]) stopShardProcessors() {
	// Synchronous: collect each per-shard done channel and block until all
	// Stop() calls finish.
	var doneChans []<-chan struct{}
	e.managedProcessors.Range(func(shardID string, _ *managedProcessor[SP]) bool {
		if ch := e.stopManagerProcessor(shardID); ch != nil {
			doneChans = append(doneChans, ch)
		}
		return true
	})
	for _, ch := range doneChans {
		<-ch
	}
}

// addManagerProcessor creates a new shard processor, stores it in the map
// synchronously (state: starting), then fires 2 goroutines: one to call Start()
// and one per-shard timeout watcher. No-ops if the shard is already tracked.
func (e *executorImpl[SP]) addManagerProcessor(ctx context.Context, shardID string) {
	if existing, ok := e.managedProcessors.Load(shardID); ok {
		// The processor already exists. If it is stopping (Stop goroutine still
		// running) we log and skip — the next heartbeat will re-send the shard as
		// READY once the entry is gone and addManagerProcessor will create a fresh one.
		if existing.getState() == processorStateStopping {
			e.logger.Info("shard processor add skipped: existing processor is still stopping, will retry on next heartbeat",
				zap.String(tag.ShardKey, shardID))
		}
		return
	}

	e.metrics.Counter(metricsconstants.ShardDistributorExecutorShardsStarted).Inc(1)
	processor, err := e.shardProcessorFactory.NewShardProcessor(shardID)
	if err != nil {
		e.logger.Error("failed to create shard processor", zap.Error(err))
		e.metrics.Counter(metricsconstants.ShardDistributorExecutorProcessorCreationFailures).Inc(1)
		return
	}
	managedProcessor := newManagedProcessor(processor, processorStateStarting)
	e.managedProcessors.Store(shardID, managedProcessor)
	// Seed the last-use time so that the cleanup loop can evict shards that are
	// assigned but never accessed via GetShardProcess within the TTL window.
	e.processorsToLastUse.Store(shardID, e.timeSource.Now())

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := processor.Start(context.WithoutCancel(ctx)); err != nil {
			e.logger.Error("shard processor start failed", zap.String(tag.ShardKey, shardID), zap.Error(err))
			// Remove the failed processor so the next heartbeat can retry.
			e.managedProcessors.Delete(shardID)
			return
		}
		managedProcessor.setState(processorStateStarted)
	}()
	go func() {
		timer := e.timeSource.NewTimer(processorAsyncOperationTimeout)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.Chan():
			e.logger.Error("shard processor start timed out", zap.String(tag.ShardKey, shardID))
			e.metrics.Counter(metricsconstants.ShardDistributorExecutorProcessorStartTimeout).Inc(1)
		}
	}()
}

// stopManagerProcessor transitions the processor to stopping, removes it from the
// map synchronously, then fires 2 goroutines: one to call Stop() and one per-shard
// timeout watcher. Returns a done channel that closes when Stop() returns so callers
// that need to block (e.g. stopShardProcessors) can wait on it.
// No-ops if the processor is not found or already stopping; returns nil in that case.
func (e *executorImpl[SP]) stopManagerProcessor(shardID string) <-chan struct{} {
	managedProcessor, ok := e.managedProcessors.Load(shardID)
	if !ok || managedProcessor.getState() == processorStateStopping {
		return nil
	}
	e.metrics.Counter(metricsconstants.ShardDistributorExecutorShardsStopped).Inc(1)
	managedProcessor.setState(processorStateStopping)
	// Delete synchronously so the next heartbeat sees a clean state immediately
	// and can re-add the shard if the server re-assigns it.
	e.managedProcessors.Delete(shardID)

	done := make(chan struct{})
	go func() {
		defer close(done)
		managedProcessor.processor.Stop()
	}()
	go func() {
		timer := e.timeSource.NewTimer(processorAsyncOperationTimeout)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.Chan():
			e.logger.Error("shard processor stop timed out", zap.String(tag.ShardKey, shardID))
			e.metrics.Counter(metricsconstants.ShardDistributorExecutorProcessorStopTimeout).Inc(1)
		}
	}()
	return done
}

func (e *executorImpl[SP]) shardCleanUpLoop(ctx context.Context) {
	// We don't run the loop for invalid durations
	if e.ttlShard <= 0 {
		e.logger.Info("cleanUpLoop not configured", zap.Any(tag.TTLShard, e.ttlShard))
		return
	}
	e.logger.Info("cleanUpLoop configured", zap.Any(tag.TTLShard, e.ttlShard))
	shardCleanUpTimer := e.timeSource.NewTimer(backoff.JitDuration(e.ttlShard, heartbeatJitterCoeff))
	defer shardCleanUpTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			e.logger.Info("stopping cleanUpLoop: context done")
			return
		case <-e.stopC:
			e.logger.Info("stopping cleanUpLoop: stop signal")
			return
		case <-shardCleanUpTimer.Chan():
			e.logger.Info("running cleanUpLoop")
			e.processorsToLastUse.Range(func(shardID string, lastUse time.Time) bool {
				if lastUse.Add(e.ttlShard).Before(e.timeSource.Now()) && e.enabled() {
					mp, ok := e.managedProcessors.Load(shardID)
					if ok {
						e.logger.Info("shard cleanup: marking shard as done (idle past TTL)",
							zap.String(tag.ShardKey, shardID),
							zap.Any(tag.IdleDuration, e.timeSource.Since(lastUse).String()),
						)
						mp.processor.SetShardStatus(types.ShardStatusDONE)
						e.metrics.Counter(metricsconstants.ShardDistributorExecutorShardsCleanedUpDone).Inc(1)
					}
					e.processorsToLastUse.Delete(shardID)
				}
				return true
			})
			shardCleanUpTimer.Reset(backoff.JitDuration(e.ttlShard, heartbeatJitterCoeff))
		}
	}
}

func (e *executorImpl[SP]) GetNamespace() string {
	return e.namespace
}

func (e *executorImpl[SP]) GetExecutorID() string {
	return e.executorID
}

func (e *executorImpl[SP]) SetMetadata(metadata map[string]string) {
	e.metadata.Set(metadata)
}

func (e *executorImpl[SP]) GetMetadata() map[string]string {
	return e.metadata.Get()
}
