package types

import (
	"bytes"
	"encoding/hex"
	"errors"
)

// Range represents a contiguous interval of the key space [LowInclusive, HighExclusive).
// Ranges are the unit of ownership in the range-based model, replacing discrete shard assignments.
type Range struct {
	// LowInclusive is the inclusive lower bound in the ordered key space.
	LowInclusive []byte
	// HighExclusive is the exclusive upper bound in the ordered key space.
	HighExclusive []byte
}

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

// RangeUpperBoundInfinity is the exclusive upper bound of a final range, which
// extends to the end of the key space. It is one byte longer than a fingerprint
// so that byte comparison orders it after every finite fingerprint
// while it remains a valid upper bound for the half-open interval.
//
// Callers must not mutate it: IsFinalRange, NewFinalRange and the storage
// bound validation all compare against this value.
var RangeUpperBoundInfinity = bytes.Repeat([]byte{0xFF}, 9)

// NewFinalRange creates the final range [lowInclusive, infinity), covering every
// key greater than or equal to lowInclusive.
func NewFinalRange(lowInclusive []byte) (*Range, error) {
	if lowInclusive == nil {
		return nil, ErrRangeNilBounds
	}

	r := &Range{
		LowInclusive:  bytes.Clone(lowInclusive),
		HighExclusive: bytes.Clone(RangeUpperBoundInfinity),
	}

	if !r.IsValid() {
		return nil, ErrRangeInvalid
	}

	return r, nil
}

// IsValid reports whether the range is well-formed: non-nil, with non-nil bounds
// and LowInclusive strictly less than HighExclusive.
func (r *Range) IsValid() bool {
	if r == nil || r.LowInclusive == nil || r.HighExclusive == nil {
		return false
	}
	return bytes.Compare(r.LowInclusive, r.HighExclusive) < 0
}

// Contains reports whether key lies in the half-open interval
// [LowInclusive, HighExclusive). A nil key or an invalid range returns false.
func (r *Range) Contains(key []byte) bool {
	if !r.IsValid() || key == nil {
		return false
	}
	return bytes.Compare(r.LowInclusive, key) <= 0 && bytes.Compare(key, r.HighExclusive) < 0
}

// Overlaps reports whether the two ranges share at least one key.
func (r *Range) Overlaps(other *Range) bool {
	if !r.IsValid() || !other.IsValid() {
		return false
	}
	return bytes.Compare(r.LowInclusive, other.HighExclusive) < 0 &&
		bytes.Compare(other.LowInclusive, r.HighExclusive) < 0
}

// Adjacent reports whether the two ranges touch at exactly one boundary, with no
// gap and no overlap.
func (r *Range) Adjacent(other *Range) bool {
	if !r.IsValid() || !other.IsValid() {
		return false
	}
	return bytes.Equal(r.HighExclusive, other.LowInclusive) ||
		bytes.Equal(other.HighExclusive, r.LowInclusive)
}

// IsFinalRange reports whether the range extends to the end of the key space.
func (r *Range) IsFinalRange() bool {
	return r != nil && bytes.Equal(r.HighExclusive, RangeUpperBoundInfinity)
}

// String returns a hexadecimal representation suitable for logs.
func (r *Range) String() string {
	if !r.IsValid() {
		return "<invalid range>"
	}

	return "[" + hex.EncodeToString(r.LowInclusive) + ", " + hex.EncodeToString(r.HighExclusive) + ")"
}
