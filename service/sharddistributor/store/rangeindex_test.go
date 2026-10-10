package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/types"
)

func mustRange(t *testing.T, low, high []byte) *types.Range {
	t.Helper()
	r, err := types.NewRange(low, high)
	require.NoError(t, err)
	return r
}

func mustFinalRange(t *testing.T, low []byte) *types.Range {
	t.Helper()
	r, err := types.NewFinalRange(low)
	require.NoError(t, err)
	return r
}

// assignment builds a READY RangeAssignment for the given range and executor.
func assignment(t *testing.T, r *types.Range, executorID string) *RangeAssignment {
	t.Helper()
	require.NotNil(t, r)
	return &RangeAssignment{Range: r, ExecutorID: executorID, Status: types.AssignmentStatusREADY}
}

// validCover builds a three-range cover of [empty, 0x40), [0x40, 0x80) and
// [0x80, infinity), each with a distinct executor.
func validCover(t *testing.T) []*RangeAssignment {
	t.Helper()
	return []*RangeAssignment{
		assignment(t, mustRange(t, []byte{}, []byte{0x40}), "exec-1"),
		assignment(t, mustRange(t, []byte{0x40}, []byte{0x80}), "exec-2"),
		assignment(t, mustFinalRange(t, []byte{0x80}), "exec-3"),
	}
}

// permute returns a new slice with elements of ranges reordered by order.
func permute(t *testing.T, ranges []*RangeAssignment, order []int) []*RangeAssignment {
	t.Helper()
	require.Len(t, order, len(ranges))
	out := make([]*RangeAssignment, len(ranges))
	for i, j := range order {
		out[i] = ranges[j]
	}
	return out
}

// mustIndex builds a RangeIndex and fails the test if the cover is invalid.
func mustIndex(t *testing.T, ranges []*RangeAssignment) *RangeIndex {
	t.Helper()
	idx, err := NewRangeIndex(ranges)
	require.NoError(t, err)
	require.NotNil(t, idx)
	return idx
}

func executorIDs(ranges []*RangeAssignment) []string {
	ids := make([]string, 0, len(ranges))
	for _, r := range ranges {
		ids = append(ids, r.ExecutorID)
	}
	return ids
}

func TestNewRangeIndex(t *testing.T) {
	tests := []struct {
		name      string
		ranges    []*RangeAssignment
		wantOrder []string
		wantErr   string
	}{
		{
			name:      "valid cover",
			ranges:    validCover(t),
			wantOrder: []string{"exec-1", "exec-2", "exec-3"},
		},
		{
			name:      "unsorted input is sorted by lower bound",
			ranges:    permute(t, validCover(t), []int{2, 0, 1}),
			wantOrder: []string{"exec-1", "exec-2", "exec-3"},
		},
		{
			name:    "empty",
			ranges:  nil,
			wantErr: "must not be empty",
		},
		{
			name: "gap between ranges",
			ranges: []*RangeAssignment{
				assignment(t, mustRange(t, []byte{}, []byte{0x40}), "exec-1"),
				assignment(t, mustRange(t, []byte{0x60}, []byte{0x80}), "exec-2"),
				assignment(t, mustFinalRange(t, []byte{0x80}), "exec-3"),
			},
			wantErr: "not adjacent",
		},
		{
			name: "overlap between ranges",
			ranges: []*RangeAssignment{
				assignment(t, mustRange(t, []byte{}, []byte{0x40}), "exec-1"),
				assignment(t, mustRange(t, []byte{0x20}, []byte{0x80}), "exec-2"),
				assignment(t, mustFinalRange(t, []byte{0x80}), "exec-3"),
			},
			wantErr: "not adjacent",
		},
		{
			name: "first range does not start at minimum",
			ranges: []*RangeAssignment{
				assignment(t, mustRange(t, []byte{0x10}, []byte{0x40}), "exec-1"),
				assignment(t, mustRange(t, []byte{0x40}, []byte{0x80}), "exec-2"),
				assignment(t, mustFinalRange(t, []byte{0x80}), "exec-3"),
			},
			wantErr: "minimum bound",
		},
		{
			name: "last range is not final",
			ranges: []*RangeAssignment{
				assignment(t, mustRange(t, []byte{}, []byte{0x40}), "exec-1"),
				assignment(t, mustRange(t, []byte{0x40}, []byte{0x80}), "exec-2"),
				assignment(t, mustRange(t, []byte{0x80}, []byte{0xc0}), "exec-3"),
			},
			wantErr: "end of the key space",
		},
		{
			name:    "nil assignment",
			ranges:  []*RangeAssignment{nil},
			wantErr: "nil",
		},
		{
			name:    "assignment with nil range",
			ranges:  []*RangeAssignment{{Range: nil, ExecutorID: "exec-1", Status: types.AssignmentStatusREADY}},
			wantErr: "nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, err := NewRangeIndex(tt.ranges)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, idx)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, idx)
			assert.Equal(t, tt.wantOrder, executorIDs(idx.Ranges()))
		})
	}
}

func TestNewRangeIndex_DoesNotMutateInput(t *testing.T) {
	tests := []struct {
		name  string
		order []int
	}{
		{name: "already sorted", order: []int{0, 1, 2}},
		{name: "reversed", order: []int{2, 1, 0}},
		{name: "rotated", order: []int{2, 0, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := permute(t, validCover(t), tt.order)
			before := executorIDs(input)

			_ = mustIndex(t, input)

			assert.Equal(t, before, executorIDs(input))
		})
	}
}

func TestRangeIndex_GetRangeForKey(t *testing.T) {
	idx := mustIndex(t, validCover(t))

	// A valid cover contains every key, so each case is found. The
	// empty slice is the minimum key and belongs to the first range.
	tests := []struct {
		name      string
		key       []byte
		wantOwner string
	}{
		{name: "empty key is the minimum", key: []byte{}, wantOwner: "exec-1"},
		{name: "first range", key: []byte{0x00}, wantOwner: "exec-1"},
		{name: "first range upper interior", key: []byte{0x3f}, wantOwner: "exec-1"},
		{name: "middle range at lower bound", key: []byte{0x40}, wantOwner: "exec-2"},
		{name: "middle range interior", key: []byte{0x60}, wantOwner: "exec-2"},
		{name: "final range at lower bound", key: []byte{0x80}, wantOwner: "exec-3"},
		{name: "final range upper interior", key: []byte{0xff}, wantOwner: "exec-3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := idx.GetRangeForKey(tt.key)
			require.True(t, found)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantOwner, got.ExecutorID)
		})
	}
}

func TestRangeIndex_Ranges(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(got []*RangeAssignment)
		wantOrder []string
	}{
		{
			name:      "returns ranges sorted by lower bound",
			mutate:    func([]*RangeAssignment) {},
			wantOrder: []string{"exec-1", "exec-2", "exec-3"},
		},
		{
			name:      "mutating the returned slice does not affect the index",
			mutate:    func(got []*RangeAssignment) { got[0] = nil },
			wantOrder: []string{"exec-1", "exec-2", "exec-3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := mustIndex(t, validCover(t))

			tt.mutate(idx.Ranges())

			assert.Equal(t, tt.wantOrder, executorIDs(idx.Ranges()))
		})
	}
}
