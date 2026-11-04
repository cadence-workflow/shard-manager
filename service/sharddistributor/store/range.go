package store

import (
	"context"
	"fmt"

	"github.com/cadence-workflow/shard-manager/common/types"
)

// ErrRangeNotImplemented is returned by range operations that are not implemented yet.
var ErrRangeNotImplemented = fmt.Errorf("range operation not implemented")

// RangeAssignment is the internal representation of range ownership. ID
// uniquely identifies the assignment record independent of its bounds, and
// Range defines the keyspace interval it covers.
type RangeAssignment struct {
	ID         string
	Range      types.Range
	ExecutorID string
	Status     types.AssignmentStatus
	// ModRevision is the etcd modification revision of the range key, used as
	// the optimistic-lock token for guarded writes. It is 0 for a range that
	// does not yet exist in etcd and is filled by ListRanges.
	ModRevision int64
}

// RangeStore is the set of storage operations for range-based ownership.
// Embedded in Store.
type RangeStore interface {
	// ListRanges returns every range in a namespace, sorted by lower bound.
	ListRanges(ctx context.Context, namespace string) ([]*RangeAssignment, error)

	// AssignRanges atomically rewrites the full range cover in a single guarded
	// transaction, failing if the guard or any assignment's ModRevision
	// precondition fails.
	AssignRanges(ctx context.Context, namespace string, request AssignRangesRequest, guard GuardFunc) error
}

// AssignRangesRequest is the complete target cover passed to AssignRanges.
type AssignRangesRequest struct {
	Cover *RangeCover
}
