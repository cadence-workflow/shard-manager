package greedy

import (
	"time"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// PrepareAssignmentStatistics returns complete statistics maps for executors
// affected by an assignment change.
func PrepareAssignmentStatistics(
	previousOwnersByShard map[string]string,
	newAssignments map[string]store.AssignedState,
	previousStatistics map[string]store.ShardStatistics,
	now time.Time,
) []store.ExecutorShardStatistics {
	executorsAffectedByAssignmentChange := findAffectedExecutors(previousOwnersByShard, newAssignments)
	statisticsUpdatesForAffectedExecutors := buildStatisticsUpdates(
		executorsAffectedByAssignmentChange,
		previousOwnersByShard,
		newAssignments,
		previousStatistics,
		now,
	)
	return statisticsUpdatesForAffectedExecutors
}

func findAffectedExecutors(
	previousOwnersByShard map[string]string,
	newAssignments map[string]store.AssignedState,
) map[string]struct{} {
	executorsAffectedByAssignmentChange := make(map[string]struct{})
	for newOwnerID, assignedState := range newAssignments {
		for shardID := range assignedState.AssignedShards {
			previousOwnerID, previouslyAssigned := previousOwnersByShard[shardID]
			if previouslyAssigned && previousOwnerID == newOwnerID {
				continue
			}

			executorsAffectedByAssignmentChange[newOwnerID] = struct{}{}
			if previouslyAssigned {
				executorsAffectedByAssignmentChange[previousOwnerID] = struct{}{}
			}
		}
	}
	return executorsAffectedByAssignmentChange
}

func buildStatisticsUpdates(
	executorsAffectedByAssignmentChange map[string]struct{},
	previousOwnersByShard map[string]string,
	newAssignments map[string]store.AssignedState,
	previousStatistics map[string]store.ShardStatistics,
	now time.Time,
) []store.ExecutorShardStatistics {
	updates := make([]store.ExecutorShardStatistics, 0, len(executorsAffectedByAssignmentChange))
	for executorID := range executorsAffectedByAssignmentChange {
		assignedState := newAssignments[executorID]
		statisticsByShard := make(map[string]store.ShardStatistics, len(assignedState.AssignedShards))
		for shardID := range assignedState.AssignedShards {
			previousOwnerID, hadPreviousOwner := previousOwnersByShard[shardID]
			shardStatistics, hasPreviousStatistics := previousStatistics[shardID]
			sameOwner := hadPreviousOwner && previousOwnerID == executorID
			hasStatisticsFromPreviousAssignment := hadPreviousOwner && hasPreviousStatistics

			if !sameOwner {
				if hasStatisticsFromPreviousAssignment {
					shardStatistics.LastMoveTime = now
				} else {
					shardStatistics = store.ShardStatistics{}
				}
			}

			// Store the zero value for unmeasured shards so a later move records LastMoveTime.
			statisticsByShard[shardID] = shardStatistics
		}
		updates = append(updates, store.ExecutorShardStatistics{
			ExecutorID: executorID,
			Statistics: statisticsByShard,
		})
	}

	return updates
}
