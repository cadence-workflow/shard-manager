package types

import (
	"bytes"
	"encoding/hex"
	"errors"
)

// Range represents a contiguous interval of the key space [LowInclusive, HighExclusive).
// Ranges are the unit of ownership in the range-based model, replacing discrete shard assignments.
// Construct Range values with NewRange or NewFinalRange. Contains, Overlaps and Adjacent
// do not validate their inputs, so call IsValid on ranges of unknown origin.
type Range struct {
	// LowInclusive is the inclusive lower bound in the ordered key space.
	LowInclusive []byte
	// HighExclusive is the exclusive upper bound in the ordered key space, or
	// nil when the range extends to the end of the key space.
	HighExclusive []byte
}

var (
	ErrRangeInvalid   = errors.New("range is invalid: LowInclusive must be non-empty and < HighExclusive")
	ErrRangeNilBounds = errors.New("range bounds cannot be nil")
)

// NewRange creates a finite range [lowInclusive, highExclusive). Both bounds
// must be non-empty. Use NewFinalRange for a range with an unbounded upper
// bound.
func NewRange(lowInclusive, highExclusive []byte) (Range, error) {
	if lowInclusive == nil || highExclusive == nil {
		return Range{}, ErrRangeNilBounds
	}
	if len(lowInclusive) == 0 || len(highExclusive) == 0 {
		return Range{}, ErrRangeInvalid
	}

	r := Range{
		LowInclusive:  bytes.Clone(lowInclusive),
		HighExclusive: bytes.Clone(highExclusive),
	}

	if !r.IsValid() {
		return Range{}, ErrRangeInvalid
	}

	return r, nil
}

// NewFinalRange creates the final range [lowInclusive, infinity), covering every
// key greater than or equal to lowInclusive.
func NewFinalRange(lowInclusive []byte) (Range, error) {
	if lowInclusive == nil {
		return Range{}, ErrRangeNilBounds
	}

	r := Range{
		LowInclusive:  bytes.Clone(lowInclusive),
		HighExclusive: nil,
	}

	if !r.IsValid() {
		return Range{}, ErrRangeInvalid
	}

	return r, nil
}

// IsValid reports whether the range is well-formed: LowInclusive is non-empty,
// and either the range is unbounded above or LowInclusive is strictly less than
// HighExclusive.
func (r Range) IsValid() bool {
	if len(r.LowInclusive) == 0 {
		return false
	}
	return r.IsFinalRange() || bytes.Compare(r.LowInclusive, r.HighExclusive) < 0
}

// Contains reports whether key lies in the half-open interval
// [LowInclusive, HighExclusive). A nil key returns false.
func (r Range) Contains(key []byte) bool {
	if key == nil {
		return false
	}
	return bytes.Compare(r.LowInclusive, key) <= 0 &&
		(r.IsFinalRange() || bytes.Compare(key, r.HighExclusive) < 0)
}

// Overlaps reports whether the two ranges share at least one key.
func (r Range) Overlaps(other Range) bool {
	return (other.IsFinalRange() || bytes.Compare(r.LowInclusive, other.HighExclusive) < 0) &&
		(r.IsFinalRange() || bytes.Compare(other.LowInclusive, r.HighExclusive) < 0)
}

// Adjacent reports whether the two ranges touch at exactly one boundary, with no
// gap and no overlap.
func (r Range) Adjacent(other Range) bool {
	return (!r.IsFinalRange() && bytes.Equal(r.HighExclusive, other.LowInclusive)) ||
		(!other.IsFinalRange() && bytes.Equal(other.HighExclusive, r.LowInclusive))
}

// IsFinalRange reports whether the range extends to the end of the key space.
func (r Range) IsFinalRange() bool {
	return len(r.HighExclusive) == 0
}

// String returns a hexadecimal representation suitable for logs.
func (r Range) String() string {
	if !r.IsValid() {
		return "<invalid range>"
	}
	high := "inf"
	if !r.IsFinalRange() {
		high = hex.EncodeToString(r.HighExclusive)
	}
	return "[" + hex.EncodeToString(r.LowInclusive) + ", " + high + ")"
}
