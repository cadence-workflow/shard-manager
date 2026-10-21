package types

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bareRange constructs a Range directly without validation, enabling tests of invalid ranges.
func bareRange(low, high byte) *Range {
	return &Range{LowInclusive: []byte{low}, HighExclusive: []byte{high}}
}

func TestNewRange(t *testing.T) {
	tests := []struct {
		name          string
		lowInclusive  []byte
		highExclusive []byte
		wantErr       error
	}{
		{name: "valid", lowInclusive: []byte{0x00, 0x10}, highExclusive: []byte{0x00, 0x20}},
		{name: "valid - bounds of different lengths", lowInclusive: []byte{0x00}, highExclusive: []byte{0x00, 0x10}},
		{name: "valid - empty low bound", lowInclusive: []byte{}, highExclusive: []byte{0x00}},
		{name: "invalid - equal bounds", lowInclusive: []byte{0x10, 0x20}, highExclusive: []byte{0x10, 0x20}, wantErr: ErrRangeInvalid},
		{name: "invalid - low greater than high", lowInclusive: []byte{0x20, 0x30}, highExclusive: []byte{0x10, 0x20}, wantErr: ErrRangeInvalid},
		{name: "invalid - nil low bound", lowInclusive: nil, highExclusive: []byte{0x10, 0x20}, wantErr: ErrRangeNilBounds},
		{name: "invalid - nil high bound", lowInclusive: []byte{0x10, 0x20}, highExclusive: nil, wantErr: ErrRangeNilBounds},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewRange(tt.lowInclusive, tt.highExclusive)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.lowInclusive, got.LowInclusive)
			assert.Equal(t, tt.highExclusive, got.HighExclusive)
		})
	}
}

func TestRange_IsValid(t *testing.T) {
	tests := []struct {
		name string
		r    *Range
		want bool
	}{
		{name: "nil range", r: nil, want: false},
		{name: "valid", r: &Range{LowInclusive: []byte{0x00, 0x10}, HighExclusive: []byte{0x00, 0x20}}, want: true},
		{name: "equal bounds", r: &Range{LowInclusive: []byte{0x10}, HighExclusive: []byte{0x10}}, want: false},
		{name: "low greater than high", r: &Range{LowInclusive: []byte{0x20}, HighExclusive: []byte{0x10}}, want: false},
		{name: "nil low bound", r: &Range{LowInclusive: nil, HighExclusive: []byte{0x10}}, want: false},
		{name: "nil high bound", r: &Range{LowInclusive: []byte{0x10}, HighExclusive: nil}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.r.IsValid())
		})
	}
}

func TestRange_Contains(t *testing.T) {
	r := &Range{LowInclusive: []byte{0x10, 0x00}, HighExclusive: []byte{0x20, 0x00}}

	tests := []struct {
		name string
		key  []byte
		want bool
	}{
		{name: "key at the inclusive lower bound", key: []byte{0x10, 0x00}, want: true},
		{name: "key in the middle", key: []byte{0x15, 0x00}, want: true},
		{name: "key just below the exclusive upper bound", key: []byte{0x1F, 0xFF}, want: true},
		{name: "key at the exclusive upper bound", key: []byte{0x20, 0x00}, want: false},
		{name: "key below the range", key: []byte{0x0F, 0xFF}, want: false},
		{name: "key above the range", key: []byte{0x20, 0x01}, want: false},
		{name: "nil key", key: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, r.Contains(tt.key))
		})
	}

	assert.False(t, bareRange(0x20, 0x10).Contains([]byte{0x15}), "an invalid range contains nothing")
}

func TestRange_Overlaps(t *testing.T) {
	tests := []struct {
		name string
		r1   *Range
		r2   *Range
		want bool
	}{
		{name: "overlapping", r1: bareRange(0x10, 0x30), r2: bareRange(0x20, 0x40), want: true},
		{name: "adjacent - no overlap", r1: bareRange(0x10, 0x20), r2: bareRange(0x20, 0x30), want: false},
		{name: "disjoint", r1: bareRange(0x10, 0x20), r2: bareRange(0x30, 0x40), want: false},
		{name: "one contains the other", r1: bareRange(0x10, 0x50), r2: bareRange(0x20, 0x30), want: true},
		{name: "identical", r1: bareRange(0x10, 0x20), r2: bareRange(0x10, 0x20), want: true},
		{name: "invalid r1", r1: bareRange(0x20, 0x10), r2: bareRange(0x15, 0x25), want: false},
		{name: "invalid r2", r1: bareRange(0x10, 0x20), r2: bareRange(0x20, 0x10), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.r1.Overlaps(tt.r2))
			assert.Equal(t, tt.want, tt.r2.Overlaps(tt.r1), "Overlaps must be symmetric")
		})
	}
}

func TestRange_Adjacent(t *testing.T) {
	tests := []struct {
		name string
		r1   *Range
		r2   *Range
		want bool
	}{
		{name: "r1 immediately before r2", r1: bareRange(0x10, 0x20), r2: bareRange(0x20, 0x30), want: true},
		{name: "r2 immediately before r1", r1: bareRange(0x30, 0x40), r2: bareRange(0x10, 0x30), want: true},
		{name: "gap", r1: bareRange(0x10, 0x20), r2: bareRange(0x30, 0x40), want: false},
		{name: "overlap", r1: bareRange(0x10, 0x30), r2: bareRange(0x20, 0x40), want: false},
		{name: "invalid r1", r1: bareRange(0x20, 0x10), r2: bareRange(0x10, 0x20), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.r1.Adjacent(tt.r2))
		})
	}
}

func TestRange_String(t *testing.T) {
	tests := []struct {
		name string
		r    *Range
		want string
	}{
		{name: "valid range", r: &Range{LowInclusive: []byte{0x10, 0x20}, HighExclusive: []byte{0x30, 0x40}}, want: "[1020, 3040)"},
		{name: "invalid range", r: bareRange(0x20, 0x10), want: "<invalid range>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.r.String())
		})
	}
}

func TestRangeUpperBoundInfinity(t *testing.T) {
	maxFingerprint := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

	// The sentinel is one byte longer than a fingerprint, so it sorts after the
	// maximum fingerprint while staying a valid exclusive upper bound.
	assert.Greater(t, len(RangeUpperBoundInfinity), len(maxFingerprint))
	assert.Negative(t, bytes.Compare(maxFingerprint, RangeUpperBoundInfinity))
}

// A final range's upper bound must never share backing bytes with the package
// sentinel. Ranges are passed between components, and an in-place write to one
// bound would otherwise corrupt final-range detection for the whole process.
func TestFinalRangeDoesNotAliasSentinel(t *testing.T) {
	original := bytes.Clone(RangeUpperBoundInfinity)
	low := []byte{0x40, 0, 0, 0, 0, 0, 0, 0}

	final, err := NewFinalRange(low)
	require.NoError(t, err)
	require.True(t, final.IsFinalRange())

	final.HighExclusive[0] = 0

	assert.Equal(t, original, RangeUpperBoundInfinity, "mutating a returned bound must not change the sentinel")

	again, err := NewFinalRange(low)
	require.NoError(t, err)
	assert.True(t, again.IsFinalRange(), "the sentinel must still identify final ranges")
}

func TestNewFinalRange(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		got, err := NewFinalRange([]byte{0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		require.NoError(t, err)
		assert.True(t, got.IsValid())
		assert.True(t, got.IsFinalRange())
		// No finite upper bound covers the maximum fingerprint, so the final
		// range must.
		assert.True(t, got.Contains([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}))
	})

	t.Run("nil low bound", func(t *testing.T) {
		got, err := NewFinalRange(nil)
		assert.ErrorIs(t, err, ErrRangeNilBounds)
		assert.Nil(t, got)
	})
}

func TestRange_IsFinalRange(t *testing.T) {
	final, err := NewFinalRange([]byte{0x00})
	require.NoError(t, err)
	assert.True(t, final.IsFinalRange())

	finite, err := NewRange([]byte{0x00}, []byte{0x40})
	require.NoError(t, err)
	assert.False(t, finite.IsFinalRange())

	assert.False(t, (*Range)(nil).IsFinalRange())
}
