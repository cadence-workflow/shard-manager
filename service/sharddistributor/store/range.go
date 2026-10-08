package store

import (
	"context"
	"fmt"

	"github.com/cadence-workflow/shard-manager/common/types"
)

// ErrRangeNotFound is returned when a range with the requested lower bound does
// not exist.
var ErrRangeNotFound = fmt.Errorf("range not found")

// RangeAssignment is the internal representation of a range's ownership. The
// Range identifies the keyspace interval.
type RangeAssignment struct {
	Range      *types.Range
	ExecutorID string
	Status     types.AssignmentStatus
}

// RangeStore is the set of storage operations for range-based ownership. It is
// kept separate from Store so the range keyspace can be introduced additively
// without changing the existing shard-based contract.
type RangeStore interface {
	// PutRange stores a single range assignment. The range's lower bound
	// determines its storage key.
	PutRange(ctx context.Context, namespace string, assignment *RangeAssignment) error

	// GetRange retrieves the range whose lower bound is lowInclusive. It returns
	// ErrRangeNotFound if no such range exists.
	GetRange(ctx context.Context, namespace string, lowInclusive []byte) (*RangeAssignment, error)

	// DeleteRange removes the range whose lower bound is lowInclusive. Deleting a
	// range that does not exist is a no-op.
	DeleteRange(ctx context.Context, namespace string, lowInclusive []byte) error

	// ListRanges returns every range in a namespace, sorted by lower bound.
	ListRanges(ctx context.Context, namespace string) ([]*RangeAssignment, error)

	// TODO: Add AssignRanges for transactional multi-range writes.
	// Needed for Phase 2 split/merge, where a single range becomes two
	// and the change must be atomic. Static ranges are written once,
	// so per-range CRUD is sufficient for now.
	AssignRanges(ctx context.Context, namespace string, assignments []*RangeAssignment) error
}
