package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/hash"
	"github.com/cadence-workflow/shard-manager/common/types"
)

func TestValidateRangeBounds(t *testing.T) {
	minBound := make([]byte, hash.FingerprintSize)
	midBound := []byte{0x40, 0, 0, 0, 0, 0, 0, 0}
	highBound := []byte{0x80, 0, 0, 0, 0, 0, 0, 0}

	tests := []struct {
		name    string
		r       *types.Range
		wantErr string
	}{
		{name: "finite", r: &types.Range{LowInclusive: minBound, HighExclusive: midBound}},
		{name: "final", r: &types.Range{LowInclusive: midBound, HighExclusive: types.RangeUpperBoundInfinity}},
		{name: "minimum to infinity", r: &types.Range{LowInclusive: minBound, HighExclusive: types.RangeUpperBoundInfinity}},
		{name: "nil", r: nil, wantErr: "must not be nil"},
		{name: "equal bounds", r: &types.Range{LowInclusive: midBound, HighExclusive: midBound}, wantErr: "low must be < high"},
		{name: "inverted bounds", r: &types.Range{LowInclusive: highBound, HighExclusive: midBound}, wantErr: "low must be < high"},
		{name: "short low bound", r: &types.Range{LowInclusive: midBound[:4], HighExclusive: highBound}, wantErr: "low bound must be exactly"},
		{name: "short high bound", r: &types.Range{LowInclusive: minBound, HighExclusive: highBound[:4]}, wantErr: "high bound must be exactly"},
		{name: "long non-sentinel high bound", r: &types.Range{LowInclusive: minBound, HighExclusive: append(append([]byte{}, highBound...), 0)}, wantErr: "high bound must be exactly"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRangeBounds(tt.r)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}
