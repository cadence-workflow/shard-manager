package greedy

import (
	"time"

	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/statistics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// PrepareShardStatistics filters an executor's heartbeat reports against its
// assignment snapshot and returns the complete statistics map to persist.
func PrepareShardStatistics(
	cfg config.LoadBalancingGreedyConfig,
	namespace string,
	executorID string,
	reportedShards map[string]*types.ShardStatusReport,
	assignedState *store.AssignedState,
	previousStats map[string]store.ShardStatistics,
	now time.Time,
	logger log.Logger,
) (map[string]store.ShardStatistics, bool) {
	// The statistics key stores the executor's full map. Keep existing statistics
	// only for shards in this assignment snapshot.
	updatedStats := make(map[string]store.ShardStatistics, len(assignedState.AssignedShards))
	for shardID := range assignedState.AssignedShards {
		if previous, ok := previousStats[shardID]; ok {
			updatedStats[shardID] = previous
		}
	}

	updated := false
	smoothingTimeConstant := loadSmoothingTimeConstant(cfg, namespace)
	// An executor may report a shard until it receives the next assignment
	// response. Only assigned, non-nil reports can update statistics.
	for shardID, report := range reportedShards {
		if report == nil {
			logger.Warn("empty report, skipping smoothed load update",
				tag.ShardNamespace(namespace),
				tag.ShardExecutor(executorID),
				tag.ShardKey(shardID),
			)
			continue
		}
		if _, assigned := assignedState.AssignedShards[shardID]; !assigned {
			continue
		}

		previous := previousStats[shardID]
		updatedStatistic, err := updateShardStatistic(
			report.ShardLoad,
			previous,
			now,
			smoothingTimeConstant,
		)
		if err != nil {
			logger.Error("failed to calculate smoothed load",
				tag.Error(err),
				tag.ShardNamespace(namespace),
				tag.ShardExecutor(executorID),
				tag.ShardKey(shardID),
			)
			continue
		}

		updatedStats[shardID] = updatedStatistic
		updated = true
	}

	return updatedStats, updated
}

func updateShardStatistic(
	shardLoad float64,
	previous store.ShardStatistics,
	now time.Time,
	smoothingTimeConstant time.Duration,
) (store.ShardStatistics, error) {
	newSmoothed, err := statistics.CalculateSmoothedLoad(
		previous.SmoothedLoad,
		shardLoad,
		previous.LastUpdateTime,
		now,
		smoothingTimeConstant,
	)
	if err != nil {
		return store.ShardStatistics{}, err
	}

	return store.ShardStatistics{
		SmoothedLoad:   newSmoothed,
		LastUpdateTime: now,
		LastMoveTime:   previous.LastMoveTime,
	}, nil
}

func loadSmoothingTimeConstant(cfg config.LoadBalancingGreedyConfig, namespace string) time.Duration {
	if cfg.LoadSmoothingTimeConstant == nil {
		return statistics.DefaultLoadSmoothingTimeConstant
	}
	return cfg.LoadSmoothingTimeConstant(namespace)
}
