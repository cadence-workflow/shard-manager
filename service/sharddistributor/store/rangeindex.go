package store

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// RangeIndex is an immutable, sorted index of range assignments, ordered by
// lower bound. It answers "which range contains this key?" in O(log n).
//
// A RangeIndex can only be constructed through NewRangeIndex, which guarantees
// that its ranges form a complete, non-overlapping cover of the key space.
// The *RangeAssignment elements are shared with the caller and must not be
// mutated.
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
	}

	sorted := slices.Clone(ranges)
	slices.SortFunc(sorted, compareRangeAssignments)
	if err := validateSortedCover(sorted); err != nil {
		return nil, err
	}
	return &RangeIndex{ranges: sorted}, nil
}

// TODO: Add split/merge support. Both must return a new RangeIndex so that
// immutability is preserved:
//   - SplitRange(lowInclusive, splitKey) — split one range into two at splitKey
//   - MergeRanges(low1, low2) — merge two adjacent ranges into one

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

// validateSortedCover checks that sorted ranges form a complete cover. It
// assumes the input is already sorted by lower bound and contains no nils.
func validateSortedCover(sorted []*RangeAssignment) error {
	if len(sorted) == 0 {
		return errors.New("range cover must not be empty")
	}

	// The first range must start at the beginning of the key space. The minimum
	// bound is the empty byte string, which sorts before every fingerprint.
	if len(sorted[0].Range.LowInclusive) != 0 {
		return fmt.Errorf("first range must start at the minimum bound, got %v", sorted[0].Range)
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
