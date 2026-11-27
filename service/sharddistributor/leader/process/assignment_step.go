package process

import (
	"cmp"
	"maps"
	"slices"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// assignmentStep is the part of a planned distribution that one AssignShards request can hold.
type assignmentStep struct {
	// changedAssignments holds the new shards of every executor this step writes.
	changedAssignments map[string][]string
	// executorsToDelete are the stale executors that hold no shards after this step.
	executorsToDelete []string
	// deferredShards counts the shard changes left for a later step.
	deferredShards int
}

// shardChange moves one shard from its stored owner to its planned owner.
type shardChange struct {
	shardID string
	// from loses the shard. It is empty when the shard has no stored owner.
	from string
	// to gains the shard. It is empty when no executor should own the shard anymore.
	to string
	// needsNewOwner is true when the shard has no stored owner or its stored owner is not assignable.
	needsNewOwner bool
}

// nextAssignmentStep returns the changes from storedAssignments toward plannedAssignments that fit in
// maxChangedExecutors writes and deletes. It moves whole shards, the ones that need a new owner first,
// and leaves the rest for the next step. An executor missing from plannedAssignments loses its
// shards, and a stale executor is deleted once it holds no shards.
func nextAssignmentStep(
	storedAssignments map[string]store.AssignedState,
	plannedAssignments map[string][]string,
	assignableExecutors map[string]struct{},
	staleExecutors map[string]struct{},
	maxChangedExecutors int,
) assignmentStep {
	shardChanges := findShardChanges(storedAssignments, plannedAssignments, assignableExecutors)
	slices.SortFunc(shardChanges, needsNewOwnerFirst)

	shardsByExecutor := make(map[string]map[string]struct{}, len(storedAssignments))
	for executorID, assignedState := range storedAssignments {
		shards := make(map[string]struct{}, len(assignedState.AssignedShards))
		for shardID := range assignedState.AssignedShards {
			shards[shardID] = struct{}{}
		}
		shardsByExecutor[executorID] = shards
	}

	nextStep := assignmentStep{}
	changedExecutors := make(map[string]struct{})
	for _, change := range shardChanges {
		executorsAfterChange := len(changedExecutors) + countExecutorsNotYetChanged(changedExecutors, change)
		if executorsAfterChange > maxChangedExecutors {
			nextStep.deferredShards++
			continue
		}
		addChangedExecutors(changedExecutors, change)
		applyShardChange(shardsByExecutor, change)
	}

	// Executors without shards still get an assignment record.
	for executorID := range assignableExecutors {
		_, hasAssignment := shardsByExecutor[executorID]
		if hasAssignment || len(changedExecutors) >= maxChangedExecutors {
			continue
		}
		changedExecutors[executorID] = struct{}{}
		shardsByExecutor[executorID] = make(map[string]struct{})
	}

	for executorID := range staleExecutors {
		if len(shardsByExecutor[executorID]) > 0 {
			continue
		}
		_, alreadyChanged := changedExecutors[executorID]
		if !alreadyChanged && len(changedExecutors) >= maxChangedExecutors {
			continue
		}
		changedExecutors[executorID] = struct{}{}
		nextStep.executorsToDelete = append(nextStep.executorsToDelete, executorID)
		delete(shardsByExecutor, executorID)
	}

	nextStep.changedAssignments = make(map[string][]string, len(changedExecutors))
	for executorID := range changedExecutors {
		// Executors deleted in this step have no assignment left to write.
		shards, hasAssignment := shardsByExecutor[executorID]
		if !hasAssignment {
			continue
		}
		nextStep.changedAssignments[executorID] = slices.Collect(maps.Keys(shards))
	}
	return nextStep
}

// findShardChanges returns a change for every shard whose planned owner differs from its stored owner.
func findShardChanges(
	storedAssignments map[string]store.AssignedState,
	plannedAssignments map[string][]string,
	assignableExecutors map[string]struct{},
) []shardChange {
	storedOwnersByShard := make(map[string]string)
	for executorID, assignedState := range storedAssignments {
		for shardID := range assignedState.AssignedShards {
			storedOwnersByShard[shardID] = executorID
		}
	}
	plannedOwnersByShard := make(map[string]string)
	for executorID, shards := range plannedAssignments {
		for _, shardID := range shards {
			plannedOwnersByShard[shardID] = executorID
		}
	}

	var shardChanges []shardChange
	// Shards with a stored owner that move to another owner or that no executor should own anymore,
	// such as a shard that was just drained. plannedOwner is empty for those.
	for shardID, storedOwner := range storedOwnersByShard {
		plannedOwner := plannedOwnersByShard[shardID]
		if storedOwner == plannedOwner {
			continue
		}
		_, storedOwnerIsAssignable := assignableExecutors[storedOwner]
		change := shardChange{
			shardID:       shardID,
			from:          storedOwner,
			to:            plannedOwner,
			needsNewOwner: !storedOwnerIsAssignable,
		}
		shardChanges = append(shardChanges, change)
	}

	// Shards with a planned owner but no stored owner, such as a shard that was just undrained.
	for shardID, plannedOwner := range plannedOwnersByShard {
		if _, hasStoredOwner := storedOwnersByShard[shardID]; hasStoredOwner {
			continue
		}
		change := shardChange{
			shardID:       shardID,
			from:          "",
			to:            plannedOwner,
			needsNewOwner: true,
		}
		shardChanges = append(shardChanges, change)
	}
	return shardChanges
}

// needsNewOwnerFirst orders the changes that need a new owner first, then by shard ID.
func needsNewOwnerFirst(a, b shardChange) int {
	if a.needsNewOwner != b.needsNewOwner {
		if a.needsNewOwner {
			return -1
		}
		return 1
	}
	return cmp.Compare(a.shardID, b.shardID)
}

func countExecutorsNotYetChanged(changedExecutors map[string]struct{}, change shardChange) int {
	notYetChanged := 0
	if _, ok := changedExecutors[change.from]; change.from != "" && !ok {
		notYetChanged++
	}
	if _, ok := changedExecutors[change.to]; change.to != "" && !ok {
		notYetChanged++
	}
	return notYetChanged
}

func addChangedExecutors(changedExecutors map[string]struct{}, change shardChange) {
	if change.from != "" {
		changedExecutors[change.from] = struct{}{}
	}
	if change.to != "" {
		changedExecutors[change.to] = struct{}{}
	}
}

func applyShardChange(shardsByExecutor map[string]map[string]struct{}, change shardChange) {
	if change.from != "" {
		delete(shardsByExecutor[change.from], change.shardID)
	}
	if change.to == "" {
		return
	}
	if shardsByExecutor[change.to] == nil {
		shardsByExecutor[change.to] = make(map[string]struct{})
	}
	shardsByExecutor[change.to][change.shardID] = struct{}{}
}
