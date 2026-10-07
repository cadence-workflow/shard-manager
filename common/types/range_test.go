package types

import (
	"bytes"
	"testing"
)

func TestNewRange(t *testing.T) {
	tests := []struct {
		name          string
		lowInclusive  []byte
		highExclusive []byte
		wantErr       bool
		errType       error
	}{
		{
			name:          "valid range",
			lowInclusive:  []byte{0x00, 0x10},
			highExclusive: []byte{0x00, 0x20},
			wantErr:       false,
		},
		{
			name:          "valid range with different lengths",
			lowInclusive:  []byte{0x00},
			highExclusive: []byte{0x00, 0x10},
			wantErr:       false,
		},
		{
			name:          "invalid - equal bounds",
			lowInclusive:  []byte{0x10, 0x20},
			highExclusive: []byte{0x10, 0x20},
			wantErr:       true,
			errType:       ErrRangeInvalid,
		},
		{
			name:          "invalid - low > high",
			lowInclusive:  []byte{0x20, 0x30},
			highExclusive: []byte{0x10, 0x20},
			wantErr:       true,
			errType:       ErrRangeInvalid,
		},
		{
			name:          "invalid - nil low bound",
			lowInclusive:  nil,
			highExclusive: []byte{0x10, 0x20},
			wantErr:       true,
			errType:       ErrRangeNilBounds,
		},
		{
			name:          "invalid - nil high bound",
			lowInclusive:  []byte{0x10, 0x20},
			highExclusive: nil,
			wantErr:       true,
			errType:       ErrRangeNilBounds,
		},
		{
			name:          "invalid - both nil",
			lowInclusive:  nil,
			highExclusive: nil,
			wantErr:       true,
			errType:       ErrRangeNilBounds,
		},
		{
			name:          "valid - empty slices",
			lowInclusive:  []byte{},
			highExclusive: []byte{0x00},
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewRange(tt.lowInclusive, tt.highExclusive)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewRange() expected error, got nil")
				}
				if tt.errType != nil && err != tt.errType {
					t.Errorf("NewRange() error = %v, want %v", err, tt.errType)
				}
				if got != nil {
					t.Errorf("NewRange() expected nil range on error, got %v", got)
				}
			} else {
				if err != nil {
					t.Errorf("NewRange() unexpected error: %v", err)
				}
				if got == nil {
					t.Errorf("NewRange() expected non-nil range")
				}
			}
		})
	}
}

func TestRange_IsValid(t *testing.T) {
	tests := []struct {
		name string
		r    *Range
		want bool
	}{
		{
			name: "nil range",
			r:    nil,
			want: false,
		},
		{
			name: "valid range",
			r: &Range{
				LowInclusive:  []byte{0x00, 0x10},
				HighExclusive: []byte{0x00, 0x20},
			},
			want: true,
		},
		{
			name: "invalid - equal bounds",
			r: &Range{
				LowInclusive:  []byte{0x10, 0x20},
				HighExclusive: []byte{0x10, 0x20},
			},
			want: false,
		},
		{
			name: "invalid - low > high",
			r: &Range{
				LowInclusive:  []byte{0x20},
				HighExclusive: []byte{0x10},
			},
			want: false,
		},
		{
			name: "invalid - nil low",
			r: &Range{
				LowInclusive:  nil,
				HighExclusive: []byte{0x10},
			},
			want: false,
		},
		{
			name: "invalid - nil high",
			r: &Range{
				LowInclusive:  []byte{0x10},
				HighExclusive: nil,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRange_Contains(t *testing.T) {
	r := &Range{
		LowInclusive:  []byte{0x10, 0x00},
		HighExclusive: []byte{0x20, 0x00},
	}

	tests := []struct {
		name string
		key  []byte
		want bool
	}{
		{
			name: "key at lower bound (inclusive)",
			key:  []byte{0x10, 0x00},
			want: true,
		},
		{
			name: "key in middle",
			key:  []byte{0x15, 0x00},
			want: true,
		},
		{
			name: "key just before upper bound",
			key:  []byte{0x1F, 0xFF},
			want: true,
		},
		{
			name: "key at upper bound (exclusive)",
			key:  []byte{0x20, 0x00},
			want: false,
		},
		{
			name: "key below range",
			key:  []byte{0x0F, 0xFF},
			want: false,
		},
		{
			name: "key above range",
			key:  []byte{0x20, 0x01},
			want: false,
		},
		{
			name: "nil key",
			key:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.Contains(tt.key); got != tt.want {
				t.Errorf("Contains(%v) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}

	// Test with invalid range
	t.Run("invalid range", func(t *testing.T) {
		invalidRange := &Range{
			LowInclusive:  []byte{0x20},
			HighExclusive: []byte{0x10},
		}
		if invalidRange.Contains([]byte{0x15}) {
			t.Error("Contains() on invalid range should return false")
		}
	})
}

func TestRange_Overlaps(t *testing.T) {
	tests := []struct {
		name string
		r1   *Range
		r2   *Range
		want bool
	}{
		{
			name: "overlapping ranges",
			r1: &Range{
				LowInclusive:  []byte{0x10},
				HighExclusive: []byte{0x30},
			},
			r2: &Range{
				LowInclusive:  []byte{0x20},
				HighExclusive: []byte{0x40},
			},
			want: true,
		},
		{
			name: "adjacent ranges (no overlap)",
			r1: &Range{
				LowInclusive:  []byte{0x10},
				HighExclusive: []byte{0x20},
			},
			r2: &Range{
				LowInclusive:  []byte{0x20},
				HighExclusive: []byte{0x30},
			},
			want: false,
		},
		{
			name: "disjoint ranges",
			r1: &Range{
				LowInclusive:  []byte{0x10},
				HighExclusive: []byte{0x20},
			},
			r2: &Range{
				LowInclusive:  []byte{0x30},
				HighExclusive: []byte{0x40},
			},
			want: false,
		},
		{
			name: "one contains the other",
			r1: &Range{
				LowInclusive:  []byte{0x10},
				HighExclusive: []byte{0x50},
			},
			r2: &Range{
				LowInclusive:  []byte{0x20},
				HighExclusive: []byte{0x30},
			},
			want: true,
		},
		{
			name: "identical ranges",
			r1: &Range{
				LowInclusive:  []byte{0x10},
				HighExclusive: []byte{0x20},
			},
			r2: &Range{
				LowInclusive:  []byte{0x10},
				HighExclusive: []byte{0x20},
			},
			want: true,
		},
		{
			name: "invalid r1",
			r1: &Range{
				LowInclusive:  []byte{0x20},
				HighExclusive: []byte{0x10},
			},
			r2: &Range{
				LowInclusive:  []byte{0x15},
				HighExclusive: []byte{0x25},
			},
			want: false,
		},
		{
			name: "invalid r2",
			r1: &Range{
				LowInclusive:  []byte{0x10},
				HighExclusive: []byte{0x20},
			},
			r2: &Range{
				LowInclusive:  []byte{0x20},
				HighExclusive: []byte{0x10},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r1.Overlaps(tt.r2); got != tt.want {
				t.Errorf("Overlaps() = %v, want %v", got, tt.want)
			}
			// Test symmetry
			if got := tt.r2.Overlaps(tt.r1); got != tt.want {
				t.Errorf("Overlaps() symmetric check = %v, want %v", got, tt.want)
			}
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
		{
			name: "adjacent - r1 before r2",
			r1: &Range{
				LowInclusive:  []byte{0x10},
				HighExclusive: []byte{0x20},
			},
			r2: &Range{
				LowInclusive:  []byte{0x20},
				HighExclusive: []byte{0x30},
			},
			want: true,
		},
		{
			name: "adjacent - r2 before r1",
			r1: &Range{
				LowInclusive:  []byte{0x30},
				HighExclusive: []byte{0x40},
			},
			r2: &Range{
				LowInclusive:  []byte{0x10},
				HighExclusive: []byte{0x30},
			},
			want: true,
		},
		{
			name: "not adjacent - gap",
			r1: &Range{
				LowInclusive:  []byte{0x10},
				HighExclusive: []byte{0x20},
			},
			r2: &Range{
				LowInclusive:  []byte{0x30},
				HighExclusive: []byte{0x40},
			},
			want: false,
		},
		{
			name: "not adjacent - overlap",
			r1: &Range{
				LowInclusive:  []byte{0x10},
				HighExclusive: []byte{0x30},
			},
			r2: &Range{
				LowInclusive:  []byte{0x20},
				HighExclusive: []byte{0x40},
			},
			want: false,
		},
		{
			name: "invalid r1",
			r1: &Range{
				LowInclusive:  []byte{0x20},
				HighExclusive: []byte{0x10},
			},
			r2: &Range{
				LowInclusive:  []byte{0x10},
				HighExclusive: []byte{0x20},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r1.Adjacent(tt.r2); got != tt.want {
				t.Errorf("Adjacent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRange_Compare(t *testing.T) {
	tests := []struct {
		name string
		r1   *Range
		r2   *Range
		want int
	}{
		{
			name: "r1 < r2",
			r1: &Range{
				LowInclusive:  []byte{0x10},
				HighExclusive: []byte{0x20},
			},
			r2: &Range{
				LowInclusive:  []byte{0x30},
				HighExclusive: []byte{0x40},
			},
			want: -1,
		},
		{
			name: "r1 > r2",
			r1: &Range{
				LowInclusive:  []byte{0x30},
				HighExclusive: []byte{0x40},
			},
			r2: &Range{
				LowInclusive:  []byte{0x10},
				HighExclusive: []byte{0x20},
			},
			want: 1,
		},
		{
			name: "r1 == r2",
			r1: &Range{
				LowInclusive:  []byte{0x10},
				HighExclusive: []byte{0x20},
			},
			r2: &Range{
				LowInclusive:  []byte{0x10},
				HighExclusive: []byte{0x20},
			},
			want: 0,
		},
		{
			name: "invalid r1",
			r1: &Range{
				LowInclusive:  []byte{0x20},
				HighExclusive: []byte{0x10},
			},
			r2: &Range{
				LowInclusive:  []byte{0x30},
				HighExclusive: []byte{0x40},
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r1.Compare(tt.r2); got != tt.want {
				t.Errorf("Compare() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRange_String(t *testing.T) {
	tests := []struct {
		name string
		r    *Range
		want string
	}{
		{
			name: "valid range",
			r: &Range{
				LowInclusive:  []byte{0x10, 0x20},
				HighExclusive: []byte{0x30, 0x40},
			},
			want: "[1020, 3040)",
		},
		{
			name: "invalid range",
			r: &Range{
				LowInclusive:  []byte{0x20},
				HighExclusive: []byte{0x10},
			},
			want: "<invalid range>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRangeUpperBoundInfinity(t *testing.T) {
	maxFingerprint := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

	// The sentinel must be longer than a fingerprint so that it sorts after the
	// maximum fingerprint.
	if len(RangeUpperBoundInfinity) <= len(maxFingerprint) {
		t.Errorf("RangeUpperBoundInfinity length = %d, want greater than %d", len(RangeUpperBoundInfinity), len(maxFingerprint))
	}

	// The maximum fingerprint must sort before the sentinel.
	if bytes.Compare(maxFingerprint, RangeUpperBoundInfinity) >= 0 {
		t.Errorf("max fingerprint %x must sort before sentinel %x", maxFingerprint, RangeUpperBoundInfinity)
	}
}

func TestNewFinalRange(t *testing.T) {
	tests := []struct {
		name         string
		lowInclusive []byte
		wantErr      bool
		errType      error
	}{
		{
			name:         "valid",
			lowInclusive: []byte{0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name:    "nil low bound",
			wantErr: true,
			errType: ErrRangeNilBounds,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewFinalRange(tt.lowInclusive)

			if tt.wantErr {
				if err != tt.errType {
					t.Errorf("NewFinalRange() error = %v, want %v", err, tt.errType)
				}
				if got != nil {
					t.Errorf("NewFinalRange() expected nil on error, got %v", got)
				}
				return
			}

			if err != nil {
				t.Fatalf("NewFinalRange() unexpected error: %v", err)
			}
			if !got.IsValid() {
				t.Error("NewFinalRange() produced an invalid range")
			}
			if !got.IsFinalRange() {
				t.Error("NewFinalRange() produced a range that is not final")
			}
			// The final range must contain the maximum fingerprint, which no
			// finite upper bound can cover.
			if !got.Contains([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}) {
				t.Error("final range must contain the maximum fingerprint")
			}
		})
	}
}

func TestRange_IsFinalRange(t *testing.T) {
	final, err := NewFinalRange([]byte{0x00})
	if err != nil {
		t.Fatalf("NewFinalRange() error: %v", err)
	}
	if !final.IsFinalRange() {
		t.Error("final range must report IsFinalRange() == true")
	}

	finite, err := NewRange([]byte{0x00}, []byte{0x40})
	if err != nil {
		t.Fatalf("NewRange() error: %v", err)
	}
	if finite.IsFinalRange() {
		t.Error("finite range must report IsFinalRange() == false")
	}

	var nilRange *Range
	if nilRange.IsFinalRange() {
		t.Error("nil range must report IsFinalRange() == false")
	}
}
