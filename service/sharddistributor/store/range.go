package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/cadence-workflow/shard-manager/common/hash"
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
	// ModRevision is the etcd modification revision of the range key, used as
	// the optimistic-lock token for guarded writes. It is 0 for a range that
	// does not yet exist in etcd and is filled by reads (ListRanges, GetRange).
	ModRevision int64
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

	// AssignRanges atomically rewrites the full range cover in a single guarded
	// transaction. The request carries the complete new cover, each element's
	// ModRevision acting as the optimistic-lock token (0 for a new range).

	// AssignRanges atomically rewrites the full range cover in a single guarded
	// transaction. Each assignment's ModRevision acts as its optimistic-lock token;
	// zero indicates a range that does not exist. The operation fails if the guard
	// or a revision check fails.
	AssignRanges(ctx context.Context, namespace string, request AssignRangesRequest, guard GuardFunc) error
}

// AssignRangesRequest contains the complete target range cover.
// Each assignment's ModRevision is its optimistic-lock token; zero indicates
// that the range does not yet exist and must be created.
type AssignRangesRequest struct {
	Assignments []*RangeAssignment
}

// ValidateLowBound checks that a lower bound is exactly hash.FingerprintSize
// bytes. The storage layer keys ranges by the hex-encoded lower bound, and
// ParseRangeKey only accepts an 8-byte bound, so any other width would produce
// a key that cannot round-trip.
func ValidateLowBound(lowInclusive []byte) error {
	if len(lowInclusive) != hash.FingerprintSize {
		return fmt.Errorf("low bound must be exactly %d bytes, got %d", hash.FingerprintSize, len(lowInclusive))
	}
	return nil
}

// ValidateRangeBounds checks that a range's bounds have the width the storage
// layer persists: the lower bound must be exactly hash.FingerprintSize bytes,
// and a finite upper bound must be exactly hash.FingerprintSize bytes. The
// final range's sentinel upper bound (9 bytes of 0xFF) is the only exempt.
func ValidateRangeBounds(r *types.Range) error {
	if r == nil {
		return errors.New("range must not be nil")
	}
	if err := ValidateLowBound(r.LowInclusive); err != nil {
		return err
	}
	if !r.IsFinalRange() && len(r.HighExclusive) != hash.FingerprintSize {
		return fmt.Errorf("high bound must be exactly %d bytes, got %d", hash.FingerprintSize, len(r.HighExclusive))
	}
	return nil
}
