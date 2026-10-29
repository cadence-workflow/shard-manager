package etcdtypes

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

func TestRangeAssignment_ToRangeAssignment(t *testing.T) {
	finite := &RangeAssignment{
		HighExclusiveHex: "8000000000000000",
		ExecutorID:       "host-1@uuid-1",
		Status:           types.AssignmentStatusREADY,
	}
	final := &RangeAssignment{
		HighExclusiveHex: "",
		ExecutorID:       "host-2@uuid-2",
		Status:           types.AssignmentStatusREADY,
	}
	low := []byte{0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	tests := map[string]struct {
		input     *RangeAssignment
		wantFinal bool
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
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := tc.input.ToRangeAssignment(low)
			if tc.input == nil {
				require.NoError(t, err)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, low, got.Range.LowInclusive)
			require.Equal(t, tc.wantFinal, got.Range.IsFinalRange())
			require.Equal(t, tc.input.ExecutorID, got.ExecutorID)
			require.Equal(t, tc.input.Status, got.Status)
		})
	}
}

func TestRangeAssignment_ToRangeAssignment_InvalidHighBound(t *testing.T) {
	_, err := (&RangeAssignment{
		HighExclusiveHex: "not-hex",
		ExecutorID:       "host-1@uuid-1",
		Status:           types.AssignmentStatusREADY,
	}).ToRangeAssignment([]byte{0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	require.ErrorContains(t, err, "decode high bound")
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
			wantHighHex: "8000000000000000",
		},
		"final": {
			input: &store.RangeAssignment{
				Range:      final,
				ExecutorID: "host-2@uuid-2",
				Status:     types.AssignmentStatusREADY,
			},
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
			Range:      finite,
			ExecutorID: "host-1@uuid-1",
			Status:     types.AssignmentStatusREADY,
		},
		"final": {
			Range:      final,
			ExecutorID: "host-2@uuid-2",
			Status:     types.AssignmentStatusREADY,
		},
	}

	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			stored := FromRangeAssignment(src)
			got, err := stored.ToRangeAssignment(src.Range.LowInclusive)
			require.NoError(t, err)
			require.Equal(t, src.Range.LowInclusive, got.Range.LowInclusive)
			require.Equal(t, src.Range.HighExclusive, got.Range.HighExclusive)
			require.Equal(t, src.ExecutorID, got.ExecutorID)
			require.Equal(t, src.Status, got.Status)
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
				HighExclusiveHex: "8000000000000000",
				ExecutorID:       "host-1@uuid-1",
				Status:           types.AssignmentStatusREADY,
			},
			jsonStr: `{"high_exclusive":"8000000000000000","executor_id":"host-1@uuid-1","status":"AssignmentStatusREADY"}`,
		},
		"final": {
			input: &RangeAssignment{
				HighExclusiveHex: "",
				ExecutorID:       "host-2@uuid-2",
				Status:           types.AssignmentStatusREADY,
			},
			jsonStr: `{"high_exclusive":"","executor_id":"host-2@uuid-2","status":"AssignmentStatusREADY"}`,
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
			require.Equal(t, tc.input.HighExclusiveHex, unmarshalled.HighExclusiveHex)
			require.Equal(t, tc.input.ExecutorID, unmarshalled.ExecutorID)
			require.Equal(t, tc.input.Status, unmarshalled.Status)
		})
	}
}
