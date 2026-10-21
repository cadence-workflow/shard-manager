package store

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cadence-workflow/shard-manager/common/hash"
)

// RangeIndex is an immutable sorted index of a validated range cover.
// Ranges are ordered by low bound and support O(log n) key lookup.
//
// RangeAssignment objects are shared with the caller and must not be mutated.
type RangeIndex struct {
	ranges []*RangeAssignment
}

// NewRangeIndex builds a sorted index and validates that the ranges form a
// complete, non-overlapping cover of the key space. The input slice is not
// modified or retained.
func NewRangeIndex(ranges []*RangeAssignment) (*RangeIndex, error) {
	for _, r := range ranges {
		if r == nil || r.Range == nil {
			return nil, errors.New("range cover must not contain nil assignments or nil ranges")
		}
		// A range with low >= high contains no key, so a cover containing one
		// would leave the key at that boundary unowned. Checked before the sort
		// to catch this case.
		if !r.Range.IsValid() {
			return nil, fmt.Errorf("range cover contains an invalid range: low=%x high=%x",
				r.Range.LowInclusive, r.Range.HighExclusive)
		}
	}

	sorted := slices.Clone(ranges)
	slices.SortFunc(sorted, compareRangeAssignments)
	if err := ValidateCover(sorted); err != nil {
		return nil, err
	}
	return &RangeIndex{ranges: sorted}, nil
}

func (i *RangeIndex) String() string {
	var sb strings.Builder
	sb.WriteString("RangeIndex:\n")
	for _, r := range i.ranges {
		fmt.Fprintf(&sb, "  %v -> %s (%s)\n", r.Range, r.ExecutorID, r.Status)
	}
	return sb.String()
}

// Len returns the number of ranges in the index.
func (i *RangeIndex) Len() int {
	return len(i.ranges)
}

// GetRangeForKey returns the range containing key, and whether one was found.
// Because the index is a complete cover, every key is contained in exactly one
// range, so found is always true for a valid index.
func (i *RangeIndex) GetRangeForKey(key []byte) (*RangeAssignment, bool) {
	pos, found := slices.BinarySearchFunc(i.ranges, key, func(a *RangeAssignment, k []byte) int {
		return bytes.Compare(a.Range.LowInclusive, k)
	})
	// BinarySearchFunc returns the insertion point for key. If not found, decrement
	// to find the range whose lower bound is ≤ key.
	if !found {
		pos--
	}
	if pos < 0 {
		return nil, false
	}

	r := i.ranges[pos]
	if !r.Range.Contains(key) {
		return nil, false
	}
	return r, true
}

// Ranges returns a copy of the sorted ranges in the index. The returned slice
// may be modified freely, but the *RangeAssignment elements are shared with the
// index and must not be mutated.
func (i *RangeIndex) Ranges() []*RangeAssignment {
	return slices.Clone(i.ranges)
}

// ValidateCover checks that sorted ranges form a complete cover. It assumes the
// input is already sorted by lower bound and contains no nils.
func ValidateCover(sorted []*RangeAssignment) error {
	if len(sorted) == 0 {
		return errors.New("range cover must not be empty")
	}

	// The first range must start at the minimum bound: exactly
	// hash.FingerprintSize zero bytes. The empty byte string is not a valid
	// minimum, because the storage layer only ever persists 8-byte bounds (see
	// etcdkeys.ParseRangeKey).
	if !bytes.Equal(sorted[0].Range.LowInclusive, make([]byte, hash.FingerprintSize)) {
		return fmt.Errorf("first range must start at the minimum bound of %d zero bytes, got %v", hash.FingerprintSize, sorted[0].Range)
	}

	// Adjacent ranges must share an exact boundary: the previous high bound is
	// the next low bound. A mismatch means either a gap (prev high < next low)
	// or an overlap (prev high > next low).
	for i := 1; i < len(sorted); i++ {
		prev := sorted[i-1].Range
		curr := sorted[i].Range
		if !bytes.Equal(prev.HighExclusive, curr.LowInclusive) {
			return fmt.Errorf("ranges are not adjacent (gap or overlap): %v and %v", prev, curr)
		}
	}

	if !sorted[len(sorted)-1].Range.IsFinalRange() {
		return errors.New("last range must extend to the end of the key space")
	}

	return nil
}

func compareRangeAssignments(a, b *RangeAssignment) int {
	return bytes.Compare(a.Range.LowInclusive, b.Range.LowInclusive)
}
