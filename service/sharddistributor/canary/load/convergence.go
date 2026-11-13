package load

import (
	"context"
	"slices"
	"time"

	"github.com/uber-go/tally"
	"go.uber.org/zap"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/backoff"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
	canarymetrics "github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/metrics"
)

// shardHolder is a shard and the local executor that holds it.
type shardHolder struct {
	shardKey   string
	executorID string
}

// convergenceChecker waits for the shard distributor's cache to agree with the
// local executors: the namespace state served by GetNamespaceStateIfNewer must
// assign each shard to the executor that holds it.
// A cache that misses a refresh keeps serving the previous assignments and
// shows up as a convergence timeout
type convergenceChecker struct {
	client     sharddistributor.Client
	namespace  string
	holders    func() []shardHolder
	timeout    time.Duration
	logger     *zap.Logger
	timeSource clock.TimeSource
	scope      tally.Scope
}

// await polls until the cached namespace state assigns every shard held
// locally to its holding executor, until the timeout passes, or until the
// switcher shard is drained
func (c *convergenceChecker) await(ctx context.Context, phase string) error {
	logger := c.logger.With(zap.String("phase", phase))
	scope := c.scope.Tagged(map[string]string{"phase": phase})
	start := c.timeSource.Now()

	var view assignmentView
	for {
		c.refresh(ctx, &view, logger)
		if view.switcherDrained {
			logger.Info("convergence check stopped, the switcher shard is drained")
			return nil
		}

		mismatched := c.countMismatched(view.owners)
		elapsed := c.timeSource.Since(start)
		switch {
		case mismatched == 0:
			scope.Histogram(canarymetrics.CanaryLoadConvergenceLatency, canarymetrics.CanaryLoadDurationBuckets).RecordDuration(elapsed)
			return nil
		case elapsed >= c.timeout:
			scope.Counter(canarymetrics.CanaryLoadConvergenceTimeout).Inc(1)
			logger.Warn("shard owners did not converge", zap.Int("mismatched_shards", mismatched), zap.Duration("timeout", c.timeout))
			return nil
		}

		if err := c.waitForNextPoll(ctx); err != nil {
			return err
		}
	}
}

// assignmentView is the latest namespace state received from the shard distributor
type assignmentView struct {
	revision        int64
	owners          map[string]string // shard key to executor ID
	switcherDrained bool
}

// refresh updates view if the shard distributor has a state newer than
// view.revision
func (c *convergenceChecker) refresh(ctx context.Context, view *assignmentView, logger *zap.Logger) {
	resp, err := c.client.GetNamespaceStateIfNewer(ctx, &types.GetNamespaceStateIfNewerRequest{Namespace: c.namespace, AssignmentRevision: view.revision})
	if err != nil {
		logger.Warn("namespace state poll failed", zap.Error(err))
		return
	}
	state := resp.GetState()
	if state == nil {
		return
	}
	view.revision = resp.GetAssignmentRevision()
	view.owners = shardOwners(state)
	view.switcherDrained = slices.Contains(state.GetDrainedShardKeys(), switcherShardID)
}

// countMismatched counts the locally held shards that owners does not assign
// to their holder
func (c *convergenceChecker) countMismatched(owners map[string]string) int {
	mismatched := 0
	for _, holder := range c.holders() {
		if owners[holder.shardKey] != holder.executorID {
			mismatched++
		}
	}
	return mismatched
}

// waitForNextPoll sleeps for a jittered poll interval.
// Jitter spreads the polls of canary instances that finish a scale phase together
func (c *convergenceChecker) waitForNextPoll(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.timeSource.After(backoff.JitDuration(pollInterval, pollJitterCoeff)):
		return nil
	}
}

func shardOwners(state *types.WatchNamespaceStateResponse) map[string]string {
	owners := make(map[string]string)
	for _, executor := range state.GetExecutors() {
		for _, shard := range executor.AssignedShards {
			owners[shard.GetShardKey()] = executor.GetExecutorID()
		}
	}
	return owners
}
