package store

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cadence-workflow/shard-manager/common/hash"
)

// RangeCover is an immutable, validated range cover ordered by lower bound.
type RangeCover struct {
	ranges []*RangeAssignment
}

// NewRangeCover sorts ranges and validates that they form a complete cover.
func NewRangeCover(ranges []*RangeAssignment) (*RangeCover, error) {
	for _, r := range ranges {
		if r == nil {
			return nil, errors.New("range cover must not contain nil assignments")
		}
		if !r.Range.IsValid() {
			return nil, fmt.Errorf("range cover contains an invalid range: low=%x high=%x",
				r.Range.LowInclusive, r.Range.HighExclusive)
		}
	}

	sorted := slices.Clone(ranges)
	slices.SortFunc(sorted, func(a, b *RangeAssignment) int {
		return bytes.Compare(a.Range.LowInclusive, b.Range.LowInclusive)
	})
	if err := validateCover(sorted); err != nil {
		return nil, err
	}
	return &RangeCover{ranges: sorted}, nil
}

func (c *RangeCover) String() string {
	var sb strings.Builder
	sb.WriteString("RangeCover:\n")
	for _, r := range c.ranges {
		fmt.Fprintf(&sb, "  %v -> %s (%s)\n", r.Range, r.ExecutorID, r.Status)
	}
	return sb.String()
}

// Len returns the number of ranges in the cover.
func (c *RangeCover) Len() int {
	return len(c.ranges)
}

// GetRangeForKey returns the range for a key, guaranteed to succeed for any
// valid fingerprint.
func (c *RangeCover) GetRangeForKey(key []byte) *RangeAssignment {
	pos, found := slices.BinarySearchFunc(c.ranges, key, func(a *RangeAssignment, k []byte) int {
		return bytes.Compare(a.Range.LowInclusive, k)
	})
	// A miss means key sits between two bounds, so take the lower one.
	if !found {
		pos--
	}
	return c.ranges[pos]
}

// Ranges returns a shallow copy of the sorted ranges in the cover.
func (c *RangeCover) Ranges() []*RangeAssignment {
	return slices.Clone(c.ranges)
}

func validateCover(sorted []*RangeAssignment) error {
	if len(sorted) == 0 {
		return errors.New("range cover must not be empty")
	}

	if !hash.IsMinFingerprint(sorted[0].Range.LowInclusive) {
		return fmt.Errorf("first range must start at the minimum bound, got %v", sorted[0].Range)
	}

	for i := 1; i < len(sorted); i++ {
		prev := sorted[i-1].Range
		curr := sorted[i].Range
		if prev.IsFinalRange() || !bytes.Equal(prev.HighExclusive, curr.LowInclusive) {
			return fmt.Errorf("ranges are not adjacent (gap or overlap): %v and %v", prev, curr)
		}
	}

	if !sorted[len(sorted)-1].Range.IsFinalRange() {
		return errors.New("last range must extend to the end of the key space")
	}

	return nil
}
