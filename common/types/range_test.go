package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bareRange(low, high byte) Range {
	return Range{LowInclusive: []byte{low}, HighExclusive: []byte{high}}
}

func finalRange(low byte) Range {
	return Range{LowInclusive: []byte{low}, HighExclusive: nil}
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
		{name: "invalid - empty low bound", lowInclusive: []byte{}, highExclusive: []byte{0x00}, wantErr: ErrRangeInvalid},
		{name: "invalid - empty high bound", lowInclusive: []byte{0x00}, highExclusive: []byte{}, wantErr: ErrRangeInvalid},
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
				assert.Equal(t, Range{}, got)
				assert.False(t, got.IsValid())
				return
			}

			require.NoError(t, err)
			assert.True(t, got.IsValid())
			assert.False(t, got.IsFinalRange())
			assert.Equal(t, tt.lowInclusive, got.LowInclusive)
			assert.Equal(t, tt.highExclusive, got.HighExclusive)
		})
	}

	assert.False(t, (Range{}).IsValid(), "zero-value Range must not be valid")
}

func TestNewFinalRange(t *testing.T) {
	tests := []struct {
		name         string
		lowInclusive []byte
		wantErr      error
	}{
		{name: "valid", lowInclusive: []byte{0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "invalid - empty low bound", lowInclusive: []byte{}, wantErr: ErrRangeInvalid},
		{name: "invalid - nil low bound", lowInclusive: nil, wantErr: ErrRangeNilBounds},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewFinalRange(tt.lowInclusive)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, Range{}, got)
				assert.False(t, got.IsValid())
				return
			}

			require.NoError(t, err)
			assert.True(t, got.IsValid())
			assert.True(t, got.IsFinalRange())
			assert.Nil(t, got.HighExclusive)
			assert.Equal(t, tt.lowInclusive, got.LowInclusive)
			// No finite upper bound covers the maximum fingerprint, so the final
			// range must.
			assert.True(t, got.Contains([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}))
		})
	}
}

// TestRange_IsValid covers shapes the constructors cannot produce, so the rules
// are pinned independently of NewRange and NewFinalRange.
func TestRange_IsValid(t *testing.T) {
	tests := []struct {
		name string
		r    Range
		want bool
	}{
		{name: "zero value", r: Range{}, want: false},
		{name: "empty low bound", r: Range{LowInclusive: []byte{}, HighExclusive: []byte{0x40}}, want: false},
		{name: "valid", r: Range{LowInclusive: []byte{0x40}, HighExclusive: []byte{0x80}}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.r.IsValid())
		})
	}
}

// NewRange and NewFinalRange copy their input bounds. Mutating the original
// slices must not affect the constructed Range.
func TestConstructorsCopyBounds(t *testing.T) {
	low := []byte{0x40, 0x00}
	high := []byte{0x80, 0x00}

	finite, err := NewRange(low, high)
	require.NoError(t, err)
	final, err := NewFinalRange(low)
	require.NoError(t, err)

	low[0] = 0xff
	high[0] = 0xff

	assert.Equal(t, []byte{0x40, 0x00}, finite.LowInclusive)
	assert.Equal(t, []byte{0x80, 0x00}, finite.HighExclusive)
	assert.Equal(t, []byte{0x40, 0x00}, final.LowInclusive)
	assert.Nil(t, final.HighExclusive)
}

func TestRange_Contains(t *testing.T) {
	finite := Range{LowInclusive: []byte{0x10, 0x00}, HighExclusive: []byte{0x20, 0x00}}
	final := Range{LowInclusive: []byte{0x80, 0x00}, HighExclusive: nil}

	tests := []struct {
		name string
		r    Range
		key  []byte
		want bool
	}{
		{name: "key at the inclusive lower bound", r: finite, key: []byte{0x10, 0x00}, want: true},
		{name: "key in the middle", r: finite, key: []byte{0x15, 0x00}, want: true},
		{name: "key just below the exclusive upper bound", r: finite, key: []byte{0x1F, 0xFF}, want: true},
		{name: "key at the exclusive upper bound", r: finite, key: []byte{0x20, 0x00}, want: false},
		{name: "key below the range", r: finite, key: []byte{0x0F, 0xFF}, want: false},
		{name: "key above the range", r: finite, key: []byte{0x20, 0x01}, want: false},
		{name: "nil key", r: finite, key: nil, want: false},
		{name: "final range - key at lower bound", r: final, key: []byte{0x80, 0x00}, want: true},
		{name: "final range - key above lower bound", r: final, key: []byte{0xFF, 0xFF}, want: true},
		{name: "final range - key below lower bound", r: final, key: []byte{0x7F, 0xFF}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.r.Contains(tt.key))
		})
	}
}

func TestRange_Overlaps(t *testing.T) {
	tests := []struct {
		name string
		r1   Range
		r2   Range
		want bool
	}{
		{name: "overlapping", r1: bareRange(0x10, 0x30), r2: bareRange(0x20, 0x40), want: true},
		{name: "adjacent - no overlap", r1: bareRange(0x10, 0x20), r2: bareRange(0x20, 0x30), want: false},
		{name: "disjoint", r1: bareRange(0x10, 0x20), r2: bareRange(0x30, 0x40), want: false},
		{name: "one contains the other", r1: bareRange(0x10, 0x50), r2: bareRange(0x20, 0x30), want: true},
		{name: "identical", r1: bareRange(0x10, 0x20), r2: bareRange(0x10, 0x20), want: true},
		{name: "finite overlaps final", r1: bareRange(0x10, 0x30), r2: finalRange(0x20), want: true},
		{name: "finite adjacent to final - no overlap", r1: bareRange(0x10, 0x20), r2: finalRange(0x20), want: false},
		{name: "finite disjoint from final", r1: bareRange(0x10, 0x20), r2: finalRange(0x30), want: false},
		{name: "two final ranges always overlap", r1: finalRange(0x10), r2: finalRange(0x80), want: true},
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
		r1   Range
		r2   Range
		want bool
	}{
		{name: "r1 immediately before r2", r1: bareRange(0x10, 0x20), r2: bareRange(0x20, 0x30), want: true},
		{name: "r2 immediately before r1", r1: bareRange(0x30, 0x40), r2: bareRange(0x10, 0x30), want: true},
		{name: "gap", r1: bareRange(0x10, 0x20), r2: bareRange(0x30, 0x40), want: false},
		{name: "overlap", r1: bareRange(0x10, 0x30), r2: bareRange(0x20, 0x40), want: false},
		{name: "finite immediately before final", r1: bareRange(0x10, 0x20), r2: finalRange(0x20), want: true},
		{name: "finite with gap before final", r1: bareRange(0x10, 0x20), r2: finalRange(0x30), want: false},
		{name: "two final ranges are never adjacent", r1: finalRange(0x10), r2: finalRange(0x20), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.r1.Adjacent(tt.r2))
			assert.Equal(t, tt.want, tt.r2.Adjacent(tt.r1), "Adjacent must be symmetric")
		})
	}
}

func TestRange_String(t *testing.T) {
	tests := []struct {
		name string
		r    Range
		want string
	}{
		{name: "valid finite range", r: Range{LowInclusive: []byte{0x10, 0x20}, HighExclusive: []byte{0x30, 0x40}}, want: "[1020, 3040)"},
		{name: "valid final range", r: Range{LowInclusive: []byte{0x10, 0x20}, HighExclusive: nil}, want: "[1020, inf)"},
		{name: "invalid range", r: bareRange(0x20, 0x10), want: "<invalid range>"},
		{name: "zero-value range", r: Range{}, want: "<invalid range>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.r.String())
		})
	}
}

func TestRange_IsFinalRange(t *testing.T) {
	final, err := NewFinalRange([]byte{0x00})
	require.NoError(t, err)
	assert.True(t, final.IsFinalRange())

	finite, err := NewRange([]byte{0x00}, []byte{0x40})
	require.NoError(t, err)
	assert.False(t, finite.IsFinalRange())
}
