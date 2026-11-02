package etcdtypes

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

const (
	testRangeID      = "550e8400-e29b-41d4-a716-446655440000"
	testFinalRangeID = "123e4567-e89b-12d3-a456-426614174000"
)

func TestRangeAssignment_ToRangeAssignment(t *testing.T) {
	finite := &RangeAssignment{
		LowInclusiveHex:  "4000000000000000",
		HighExclusiveHex: "8000000000000000",
		ExecutorID:       "host-1@uuid-1",
		Status:           types.AssignmentStatusREADY,
	}
	final := &RangeAssignment{
		LowInclusiveHex:  "4000000000000000",
		HighExclusiveHex: "",
		ExecutorID:       "host-2@uuid-2",
		Status:           types.AssignmentStatusREADY,
	}
	low := []byte{0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	rangeID := testRangeID

	tests := map[string]struct {
		input     *RangeAssignment
		wantFinal bool
		wantErr   string
	}{
		"nil": {
			input: nil,
		},
		"finite": {
			input:     finite,
			wantFinal: false,
		},
		"final": {
			input:     final,
			wantFinal: true,
		},
		"invalid low hex": {
			input: &RangeAssignment{
				LowInclusiveHex:  "not-hex",
				HighExclusiveHex: "8000000000000000",
			},
			wantErr: "decode low bound",
		},
		"empty low hex": {
			input: &RangeAssignment{
				LowInclusiveHex:  "",
				HighExclusiveHex: "8000000000000000",
			},
			wantErr: "build range",
		},
		"invalid high hex": {
			input: &RangeAssignment{
				LowInclusiveHex:  "4000000000000000",
				HighExclusiveHex: "not-hex",
			},
			wantErr: "decode high bound",
		},
		"inverted bounds": {
			input: &RangeAssignment{
				LowInclusiveHex:  "8000000000000000",
				HighExclusiveHex: "4000000000000000",
			},
			wantErr: "build range",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := tc.input.ToRangeAssignment(rangeID)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.Nil(t, got)
				return
			}
			if tc.input == nil {
				require.NoError(t, err)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, rangeID, got.ID)
			require.Equal(t, low, got.Range.LowInclusive)
			require.Equal(t, tc.wantFinal, got.Range.IsFinalRange())
			require.Equal(t, tc.input.ExecutorID, got.ExecutorID)
			require.Equal(t, tc.input.Status, got.Status)
		})
	}
}

func TestRangeAssignment_FromRangeAssignment(t *testing.T) {
	finite, err := types.NewRange(
		[]byte{0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		[]byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	)
	require.NoError(t, err)
	final, err := types.NewFinalRange([]byte{0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	require.NoError(t, err)

	tests := map[string]struct {
		input       *store.RangeAssignment
		wantLowHex  string
		wantHighHex string
	}{
		"nil": {
			input: nil,
		},
		"finite": {
			input: &store.RangeAssignment{
				Range:      finite,
				ExecutorID: "host-1@uuid-1",
				Status:     types.AssignmentStatusREADY,
			},
			wantLowHex:  "4000000000000000",
			wantHighHex: "8000000000000000",
		},
		"final": {
			input: &store.RangeAssignment{
				Range:      final,
				ExecutorID: "host-2@uuid-2",
				Status:     types.AssignmentStatusREADY,
			},
			wantLowHex:  "4000000000000000",
			wantHighHex: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := FromRangeAssignment(tc.input)
			if tc.input == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, tc.wantLowHex, got.LowInclusiveHex)
			require.Equal(t, tc.wantHighHex, got.HighExclusiveHex)
			require.Equal(t, tc.input.ExecutorID, got.ExecutorID)
			require.Equal(t, tc.input.Status, got.Status)
		})
	}
}

func TestRangeAssignment_RoundTrip(t *testing.T) {
	finite, err := types.NewRange(
		[]byte{0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		[]byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	)
	require.NoError(t, err)
	final, err := types.NewFinalRange([]byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	require.NoError(t, err)

	tests := map[string]*store.RangeAssignment{
		"finite": {
			ID:         testRangeID,
			Range:      finite,
			ExecutorID: "host-1@uuid-1",
			Status:     types.AssignmentStatusREADY,
		},
		"final": {
			ID:         testFinalRangeID,
			Range:      final,
			ExecutorID: "host-2@uuid-2",
			Status:     types.AssignmentStatusREADY,
		},
	}

	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			stored := FromRangeAssignment(src)
			got, err := stored.ToRangeAssignment(src.ID)
			require.NoError(t, err)
			require.Equal(t, src, got)
		})
	}
}

func TestRangeAssignment_JSONMarshalling(t *testing.T) {
	tests := map[string]struct {
		input   *RangeAssignment
		jsonStr string
	}{
		"finite": {
			input: &RangeAssignment{
				LowInclusiveHex:  "4000000000000000",
				HighExclusiveHex: "8000000000000000",
				ExecutorID:       "host-1@uuid-1",
				Status:           types.AssignmentStatusREADY,
			},
			jsonStr: `{"low_inclusive":"4000000000000000","high_exclusive":"8000000000000000","executor_id":"host-1@uuid-1","status":"AssignmentStatusREADY"}`,
		},
		"final": {
			input: &RangeAssignment{
				LowInclusiveHex:  "4000000000000000",
				HighExclusiveHex: "",
				ExecutorID:       "host-2@uuid-2",
				Status:           types.AssignmentStatusREADY,
			},
			jsonStr: `{"low_inclusive":"4000000000000000","high_exclusive":"","executor_id":"host-2@uuid-2","status":"AssignmentStatusREADY"}`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			b, err := json.Marshal(tc.input)
			require.NoError(t, err)
			require.JSONEq(t, tc.jsonStr, string(b))

			var unmarshalled RangeAssignment
			err = json.Unmarshal([]byte(tc.jsonStr), &unmarshalled)
			require.NoError(t, err)
			require.Equal(t, tc.input, &unmarshalled)
		})
	}
}
