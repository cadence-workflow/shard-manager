package types

import (
	"bytes"
	"encoding/hex"
	"errors"
)

// Range is a half-open interval [LowInclusive, HighExclusive) of the key space.
type Range struct {
	LowInclusive []byte
	// HighExclusive is nil when the range extends to the end of the key space.
	HighExclusive []byte
}

var (
	ErrRangeInvalid   = errors.New("range is invalid: LowInclusive must be non-empty and < HighExclusive")
	ErrRangeNilBounds = errors.New("range bounds cannot be nil")
)

// NewRange creates a finite range [lowInclusive, highExclusive).
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

// NewFinalRange creates an unbounded range [lowInclusive, inf).
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

// IsValid reports whether the lower bound is non-empty and precedes the upper bound.
func (r Range) IsValid() bool {
	if len(r.LowInclusive) == 0 {
		return false
	}
	return r.IsFinalRange() || bytes.Compare(r.LowInclusive, r.HighExclusive) < 0
}

// Contains reports whether key lies in [LowInclusive, HighExclusive).
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

// Adjacent reports whether the two ranges touch at one boundary.
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
