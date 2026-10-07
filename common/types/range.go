package types

import (
	"bytes"
	"encoding/hex"
	"errors"
)

// Range represents a contiguous interval of the key space [LowInclusive, HighExclusive).
// Ranges are the unit of ownership in the range-based model, replacing discrete shard assignments.
//
// Invariants:
// - LowInclusive < HighExclusive (lexicographic comparison)
// - Empty ranges are not allowed (validated by IsValid)
// - Ranges must not overlap (enforced by storage layer)
// - Complete coverage of key space required (enforced by storage layer)
type Range struct {
	// LowInclusive is the inclusive lower bound in the ordered key space.
	LowInclusive []byte
	// HighExclusive is the exclusive upper bound in the ordered key space.
	HighExclusive []byte
}

// Errors returned when constructing an invalid range.
var (
	ErrRangeInvalid   = errors.New("range is invalid: LowInclusive must be < HighExclusive")
	ErrRangeNilBounds = errors.New("range bounds cannot be nil")
)

// NewRange creates a new Range with the given bounds.
func NewRange(lowInclusive, highExclusive []byte) (*Range, error) {
	if lowInclusive == nil || highExclusive == nil {
		return nil, ErrRangeNilBounds
	}

	r := &Range{
		LowInclusive:  bytes.Clone(lowInclusive),
		HighExclusive: bytes.Clone(highExclusive),
	}

	if !r.IsValid() {
		return nil, ErrRangeInvalid
	}

	return r, nil
}

func (r *Range) IsValid() bool {
	if r == nil || r.LowInclusive == nil || r.HighExclusive == nil {
		return false
	}
	return bytes.Compare(r.LowInclusive, r.HighExclusive) < 0
}

func (r *Range) Contains(key []byte) bool {
	if !r.IsValid() || key == nil {
		return false
	}
	return bytes.Compare(r.LowInclusive, key) <= 0 && bytes.Compare(key, r.HighExclusive) < 0
}

func (r *Range) Overlaps(other *Range) bool {
	if !r.IsValid() || !other.IsValid() {
		return false
	}
	return bytes.Compare(r.LowInclusive, other.HighExclusive) < 0 &&
		bytes.Compare(other.LowInclusive, r.HighExclusive) < 0
}

// Adjacent reports whether this range and other meet at one boundary without
// requiring either range to be ordered before the other.
func (r *Range) Adjacent(other *Range) bool {
	if !r.IsValid() || !other.IsValid() {
		return false
	}
	return bytes.Equal(r.HighExclusive, other.LowInclusive) ||
		bytes.Equal(other.HighExclusive, r.LowInclusive)
}

// Compare orders valid ranges by their lower bounds. It returns:
//
//	-1 if r < other (r.LowInclusive < other.LowInclusive)
//	 0 if r == other (same bounds)
//	+1 if r > other (r.LowInclusive > other.LowInclusive)
func (r *Range) Compare(other *Range) int {
	if !r.IsValid() || !other.IsValid() {
		return 0
	}
	return bytes.Compare(r.LowInclusive, other.LowInclusive)
}

// String returns a bounded hexadecimal representation used for logs.
func (r *Range) String() string {
	if !r.IsValid() {
		return "<invalid range>"
	}

	return "[" + hex.EncodeToString(r.LowInclusive) + ", " + hex.EncodeToString(r.HighExclusive) + ")"
}
