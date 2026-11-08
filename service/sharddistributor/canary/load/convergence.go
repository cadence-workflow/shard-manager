package load

import (
	"context"
	"time"

	"github.com/uber-go/tally"
	"go.uber.org/zap"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
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
// local executors: GetShardOwner must return the executor that holds each shard.
// A cache that misses a refresh keeps serving the previous owner and shows up
// as a convergence timeout
type convergenceChecker struct {
	client     sharddistributor.Client
	namespace  string
	holders    func() []shardHolder
	timeout    time.Duration
	logger     *zap.Logger
	timeSource clock.TimeSource
	scope      tally.Scope
}

// await polls until GetShardOwner returns the holding executor for every shard
// held locally, or until the timeout passes
func (c *convergenceChecker) await(ctx context.Context, phase string) error {
	scope := c.scope.Tagged(map[string]string{"phase": phase})
	start := c.timeSource.Now()
	confirmed := make(map[shardHolder]struct{})

	ticker := c.timeSource.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		mismatched := 0
		for _, holder := range c.holders() {
			if _, ok := confirmed[holder]; ok {
				continue
			}
			resp, err := c.client.GetShardOwner(ctx, &types.GetShardOwnerRequest{Namespace: c.namespace, ShardKey: holder.shardKey})
			if err == nil && resp.Owner == holder.executorID {
				confirmed[holder] = struct{}{}
				continue
			}
			mismatched++
		}

		elapsed := c.timeSource.Since(start)
		if mismatched == 0 {
			scope.Timer(canarymetrics.CanaryLoadConvergenceLatency).Record(elapsed)
			return nil
		}
		if elapsed >= c.timeout {
			scope.Counter(canarymetrics.CanaryLoadConvergenceTimeout).Inc(1)
			c.logger.Warn("shard owners did not converge", zap.String("phase", phase), zap.Int("mismatched_shards", mismatched), zap.Duration("timeout", c.timeout))
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.Chan():
		}
	}
}
