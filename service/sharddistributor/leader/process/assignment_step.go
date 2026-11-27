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

// changedExecutorBudget counts the executors a step changes, up to what one AssignShards request holds.
// An executor counts once, however many of its shards change.
type changedExecutorBudget struct {
	changedExecutors    map[string]struct{}
	maxChangedExecutors int
}

// tryAdd counts the executors only if they all fit within the budget, and returns whether they did.
// Empty IDs are skipped.
func (b *changedExecutorBudget) tryAdd(executorIDs ...string) bool {
	executorsAfterAdd := len(b.changedExecutors)
	for _, executorID := range executorIDs {
		_, alreadyCounted := b.changedExecutors[executorID]
		if executorID != "" && !alreadyCounted {
			executorsAfterAdd++
		}
	}
	if executorsAfterAdd > b.maxChangedExecutors {
		return false
	}
	for _, executorID := range executorIDs {
		if executorID != "" {
			b.changedExecutors[executorID] = struct{}{}
		}
	}
	return true
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
	budget := changedExecutorBudget{
		changedExecutors:    make(map[string]struct{}),
		maxChangedExecutors: maxChangedExecutors,
	}
	for _, change := range shardChanges {
		if !budget.tryAdd(change.from, change.to) {
			nextStep.deferredShards++
			continue
		}
		applyShardChange(shardsByExecutor, change)
	}

	// Executors without shards still get an assignment record.
	for executorID := range assignableExecutors {
		if _, hasAssignment := shardsByExecutor[executorID]; hasAssignment {
			continue
		}
		if budget.tryAdd(executorID) {
			shardsByExecutor[executorID] = make(map[string]struct{})
		}
	}

	// Stale executors are deleted once they hold no shards.
	for executorID := range staleExecutors {
		if len(shardsByExecutor[executorID]) > 0 {
			continue
		}
		if budget.tryAdd(executorID) {
			nextStep.executorsToDelete = append(nextStep.executorsToDelete, executorID)
			delete(shardsByExecutor, executorID)
		}
	}

	nextStep.changedAssignments = make(map[string][]string, len(budget.changedExecutors))
	for executorID := range budget.changedExecutors {
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
