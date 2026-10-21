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

// b8 builds an 8-byte bound whose first byte is b and the remaining bytes are
// zero, matching the Fingerprint64 layout used for range bounds.
func b8(b byte) []byte {
	return []byte{b, 0, 0, 0, 0, 0, 0, 0}
}

// assignment builds a READY RangeAssignment for the given range and executor.
func assignment(t *testing.T, r *types.Range, executorID string) *RangeAssignment {
	t.Helper()
	require.NotNil(t, r)
	return &RangeAssignment{Range: r, ExecutorID: executorID, Status: types.AssignmentStatusREADY}
}

// validCover builds a three-range cover of [min, 0x40…), [0x40…, 0x80…) and
// [0x80…, infinity), each with a distinct executor. All bounds are 8 bytes, so
// the first low bound is the 8-zero-byte minimum.
func validCover(t *testing.T) []*RangeAssignment {
	t.Helper()
	return []*RangeAssignment{
		assignment(t, mustRange(t, b8(0x00), b8(0x40)), "exec-1"),
		assignment(t, mustRange(t, b8(0x40), b8(0x80)), "exec-2"),
		assignment(t, mustFinalRange(t, b8(0x80)), "exec-3"),
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
		// Gap between 0x40 and 0x60 leaves keys in that range unowned. This would
		// break the "complete cover" invariant and cause key lookups to fail.
		{
			name: "gap between ranges",
			ranges: []*RangeAssignment{
				assignment(t, mustRange(t, b8(0x00), b8(0x40)), "exec-1"),
				assignment(t, mustRange(t, b8(0x60), b8(0x80)), "exec-2"),
				assignment(t, mustFinalRange(t, b8(0x80)), "exec-3"),
			},
			wantErr: "not adjacent",
		},
		// Overlap means two executors claim the same keys, violating the
		// non-overlapping invariant and causing ambiguous ownership.
		{
			name: "overlap between ranges",
			ranges: []*RangeAssignment{
				assignment(t, mustRange(t, b8(0x00), b8(0x40)), "exec-1"),
				assignment(t, mustRange(t, b8(0x20), b8(0x80)), "exec-2"),
				assignment(t, mustFinalRange(t, b8(0x80)), "exec-3"),
			},
			wantErr: "not adjacent",
		},
		// The cover must start at the minimum bound (all zeros) to ensure every
		// key in the keyspace is owned. A cover starting at a higher bound leaves
		// keys below it unowned.
		{
			name: "first range has empty low bound",
			ranges: []*RangeAssignment{
				assignment(t, mustRange(t, []byte{}, b8(0x40)), "exec-1"),
				assignment(t, mustRange(t, b8(0x40), b8(0x80)), "exec-2"),
				assignment(t, mustFinalRange(t, b8(0x80)), "exec-3"),
			},
			wantErr: "minimum bound",
		},
		{
			name: "first range does not start at minimum",
			ranges: []*RangeAssignment{
				assignment(t, mustRange(t, b8(0x10), b8(0x40)), "exec-1"),
				assignment(t, mustRange(t, b8(0x40), b8(0x80)), "exec-2"),
				assignment(t, mustFinalRange(t, b8(0x80)), "exec-3"),
			},
			wantErr: "minimum bound",
		},
		// The cover must end with a final range extending to infinity. Without
		// this, keys above the last range's upper bound would be unowned.
		{
			name: "last range is not final",
			ranges: []*RangeAssignment{
				assignment(t, mustRange(t, b8(0x00), b8(0x40)), "exec-1"),
				assignment(t, mustRange(t, b8(0x40), b8(0x80)), "exec-2"),
				assignment(t, mustRange(t, b8(0x80), b8(0xc0)), "exec-3"),
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
		{
			// Degenerate range (low == high) contains no keys, leaving its boundary unowned.
			// Storage cannot produce this, but NewRangeIndex must reject it defensively.
			name: "degenerate range",
			ranges: []*RangeAssignment{
				assignment(t, mustRange(t, b8(0x00), b8(0x40)), "exec-1"),
				assignment(t, &types.Range{LowInclusive: b8(0x40), HighExclusive: b8(0x40)}, "exec-2"),
				assignment(t, mustRange(t, b8(0x40), b8(0x80)), "exec-3"),
				assignment(t, mustFinalRange(t, b8(0x80)), "exec-4"),
			},
			wantErr: "invalid range",
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

	tests := []struct {
		name      string
		key       []byte
		wantOwner string
		wantFound bool
	}{
		// The cover starts at the 8-zero-byte minimum, so the empty slice is
		// below it and is not found.
		{name: "empty key below the minimum", key: []byte{}, wantFound: false},
		{name: "minimum bound", key: b8(0x00), wantOwner: "exec-1", wantFound: true},
		{name: "first range upper interior", key: b8(0x3f), wantOwner: "exec-1", wantFound: true},
		{name: "middle range at lower bound", key: b8(0x40), wantOwner: "exec-2", wantFound: true},
		{name: "middle range interior", key: b8(0x60), wantOwner: "exec-2", wantFound: true},
		{name: "final range at lower bound", key: b8(0x80), wantOwner: "exec-3", wantFound: true},
		{name: "final range upper interior", key: b8(0xff), wantOwner: "exec-3", wantFound: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := idx.GetRangeForKey(tt.key)
			assert.Equal(t, tt.wantFound, found)
			if tt.wantFound {
				require.NotNil(t, got)
				assert.Equal(t, tt.wantOwner, got.ExecutorID)
			} else {
				assert.Nil(t, got)
			}
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
