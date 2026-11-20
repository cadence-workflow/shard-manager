package etcdtypes

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// b8 builds an 8-byte fingerprint-width bound whose first byte is b.
func b8(b byte) []byte {
	return []byte{b, 0, 0, 0, 0, 0, 0, 0}
}

func mustRange(t *testing.T, low, high []byte) types.Range {
	t.Helper()
	r, err := types.NewRange(low, high)
	require.NoError(t, err)
	return r
}

func mustFinalRange(t *testing.T, low []byte) types.Range {
	t.Helper()
	r, err := types.NewFinalRange(low)
	require.NoError(t, err)
	return r
}

func TestRangeAssignment_RoundTrip(t *testing.T) {
	tests := map[string]*store.RangeAssignment{
		"finite": {
			ID:         "range-1",
			Range:      mustRange(t, b8(0x40), b8(0x80)),
			ExecutorID: "host-1@uuid-1",
			Status:     types.AssignmentStatusREADY,
		},
		"final": {
			ID:         "range-2",
			Range:      mustFinalRange(t, b8(0x80)),
			ExecutorID: "host-2@uuid-2",
			Status:     types.AssignmentStatusREADY,
		},
		"non-fingerprint width": {
			ID:         "range-3",
			Range:      mustRange(t, []byte{0x00}, []byte{0x00, 0x10}),
			ExecutorID: "host-3@uuid-3",
			Status:     types.AssignmentStatusREADY,
		},
	}

	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := FromRangeAssignment(src).ToRangeAssignment(src.ID)
			require.NoError(t, err)
			require.Equal(t, src, got)
		})
	}
}

func TestRangeAssignment_ToRangeAssignment_Errors(t *testing.T) {
	tests := map[string]struct {
		lowHex  string
		highHex string
		wantErr string
	}{
		"invalid low hex":  {lowHex: "not-hex", highHex: "8000000000000000", wantErr: "decode low bound"},
		"empty low hex":    {lowHex: "", highHex: "8000000000000000", wantErr: "build range"},
		"invalid high hex": {lowHex: "4000000000000000", highHex: "not-hex", wantErr: "decode high bound"},
		"inverted bounds":  {lowHex: "8000000000000000", highHex: "4000000000000000", wantErr: "build range"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			stored := &RangeAssignment{LowInclusiveHex: tc.lowHex, HighExclusiveHex: tc.highHex}

			got, err := stored.ToRangeAssignment("range-1")
			require.ErrorContains(t, err, tc.wantErr)
			require.Nil(t, got)
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
			require.NoError(t, json.Unmarshal([]byte(tc.jsonStr), &unmarshalled))
			require.Equal(t, tc.input, &unmarshalled)
		})
	}
}
