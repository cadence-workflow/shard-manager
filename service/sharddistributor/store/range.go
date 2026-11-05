package store

import (
	"context"
	"fmt"

	"github.com/cadence-workflow/shard-manager/common/types"
)

// ErrRangeNotImplemented is returned by range operations that are not implemented yet.
var ErrRangeNotImplemented = fmt.Errorf("range operation not implemented")

// RangeAssignment is the internal representation of range ownership.
type RangeAssignment struct {
	ID         string
	Range      types.Range
	ExecutorID string
	Status     types.AssignmentStatus
	// ModRevision is the etcd revision of the range key; 0 means the range is new.
	ModRevision int64
}

// RangeStore is the set of storage operations for range-based ownership.
type RangeStore interface {
	// ListRanges returns every range in a namespace, sorted by lower bound.
	ListRanges(ctx context.Context, namespace string) ([]*RangeAssignment, error)

	// AssignRanges rewrites the full cover in one guarded transaction, failing on
	// a stale guard or ModRevision.
	AssignRanges(ctx context.Context, namespace string, request AssignRangesRequest, guard GuardFunc) error
}

// AssignRangesRequest is the complete target cover passed to AssignRanges.
type AssignRangesRequest struct {
	Cover *RangeCover
}
