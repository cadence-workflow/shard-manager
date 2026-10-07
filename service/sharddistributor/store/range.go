package store

import (
	"github.com/cadence-workflow/shard-manager/common/types"
)

// RangeAssignment is the internal representation of a range's ownership. The
// Range identifies the keyspace interval.
type RangeAssignment struct {
	Range      *types.Range
	ExecutorID string
	Status     types.AssignmentStatus
}
