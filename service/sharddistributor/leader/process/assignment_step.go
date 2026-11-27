package process

import (
	"cmp"
	"maps"
	"slices"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// assignmentStep is the part of a planned distribution that one AssignShards request can hold.
type assignmentStep struct {
	// assignments holds the shards after this step of every stored executor that is not deleted,
	// and of the new executors written in this step.
	assignments map[string][]string
	// executorsToDelete are the stale executors that hold no shards after this step.
	executorsToDelete []string
	// deferredShards counts the shard changes left for a later step.
	deferredShards int
}

// shardChange moves one shard from its stored owners to its planned owner.
type shardChange struct {
	shardID string
	// from are the stored owners that lose the shard.
	from []string
	// to gains the shard. It is empty when the shard is dropped, or when the planned owner already holds the shard.
	to string
	// needsNewOwner is true when no assignable executor holds the shard in storage.
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
	changes := findShardChanges(storedAssignments, plannedAssignments, assignableExecutors)
	slices.SortFunc(changes, func(a, b shardChange) int {
		if a.needsNewOwner != b.needsNewOwner {
			if a.needsNewOwner {
				return -1
			}
			return 1
		}
		return cmp.Compare(a.shardID, b.shardID)
	})

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
	for _, change := range changes {
		if len(changedExecutors)+countExecutorsNotYetChanged(changedExecutors, change) > maxChangedExecutors {
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

	nextStep.assignments = make(map[string][]string, len(shardsByExecutor))
	for executorID, shards := range shardsByExecutor {
		nextStep.assignments[executorID] = slices.Collect(maps.Keys(shards))
	}
	return nextStep
}

// findShardChanges returns a change for every shard whose planned owner differs from its stored owners.
func findShardChanges(
	storedAssignments map[string]store.AssignedState,
	plannedAssignments map[string][]string,
	assignableExecutors map[string]struct{},
) []shardChange {
	storedOwners := make(map[string][]string)
	for executorID, assignedState := range storedAssignments {
		for shardID := range assignedState.AssignedShards {
			storedOwners[shardID] = append(storedOwners[shardID], executorID)
		}
	}
	plannedOwners := make(map[string]string)
	for executorID, shards := range plannedAssignments {
		for _, shardID := range shards {
			plannedOwners[shardID] = executorID
		}
	}

	shardIDs := make(map[string]struct{}, len(storedOwners)+len(plannedOwners))
	for shardID := range storedOwners {
		shardIDs[shardID] = struct{}{}
	}
	for shardID := range plannedOwners {
		shardIDs[shardID] = struct{}{}
	}

	var changes []shardChange
	for shardID := range shardIDs {
		plannedOwner := plannedOwners[shardID]
		change := shardChange{shardID: shardID, to: plannedOwner, needsNewOwner: true}
		for _, storedOwner := range storedOwners[shardID] {
			if _, isAssignable := assignableExecutors[storedOwner]; isAssignable {
				change.needsNewOwner = false
			}
			if storedOwner == plannedOwner {
				change.to = ""
			} else {
				change.from = append(change.from, storedOwner)
			}
		}
		if len(change.from) > 0 || change.to != "" {
			changes = append(changes, change)
		}
	}
	return changes
}

func countExecutorsNotYetChanged(changedExecutors map[string]struct{}, change shardChange) int {
	notYetChanged := 0
	for _, executorID := range change.from {
		if _, ok := changedExecutors[executorID]; !ok {
			notYetChanged++
		}
	}
	if change.to != "" {
		if _, ok := changedExecutors[change.to]; !ok {
			notYetChanged++
		}
	}
	return notYetChanged
}

func addChangedExecutors(changedExecutors map[string]struct{}, change shardChange) {
	for _, executorID := range change.from {
		changedExecutors[executorID] = struct{}{}
	}
	if change.to != "" {
		changedExecutors[change.to] = struct{}{}
	}
}

func applyShardChange(shardsByExecutor map[string]map[string]struct{}, change shardChange) {
	for _, executorID := range change.from {
		delete(shardsByExecutor[executorID], change.shardID)
	}
	if change.to == "" {
		return
	}
	if shardsByExecutor[change.to] == nil {
		shardsByExecutor[change.to] = make(map[string]struct{})
	}
	shardsByExecutor[change.to][change.shardID] = struct{}{}
}
