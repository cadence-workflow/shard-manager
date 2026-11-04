package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/types"
)

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

func assignment(r types.Range, executorID string) *RangeAssignment {
	return &RangeAssignment{Range: r, ExecutorID: executorID, Status: types.AssignmentStatusREADY}
}

func validCover(t *testing.T) []*RangeAssignment {
	t.Helper()
	return []*RangeAssignment{
		assignment(mustRange(t, b8(0x00), b8(0x40)), "exec-1"),
		assignment(mustRange(t, b8(0x40), b8(0x80)), "exec-2"),
		assignment(mustFinalRange(t, b8(0x80)), "exec-3"),
	}
}

func executorIDs(ranges []*RangeAssignment) []string {
	ids := make([]string, 0, len(ranges))
	for _, r := range ranges {
		ids = append(ids, r.ExecutorID)
	}
	return ids
}

func TestNewRangeCover(t *testing.T) {
	cover := validCover(t)

	tests := []struct {
		name      string
		ranges    []*RangeAssignment
		wantOrder []string
		wantErr   string
	}{
		{name: "valid cover", ranges: cover, wantOrder: []string{"exec-1", "exec-2", "exec-3"}},
		{name: "unsorted input is sorted by lower bound", ranges: []*RangeAssignment{cover[2], cover[0], cover[1]}, wantOrder: []string{"exec-1", "exec-2", "exec-3"}},
		{name: "empty", ranges: nil, wantErr: "must not be empty"},
		{name: "nil assignment", ranges: []*RangeAssignment{nil}, wantErr: "nil"},
		{name: "invalid range", ranges: []*RangeAssignment{assignment(types.Range{}, "exec-1")}, wantErr: "invalid range"},
		{name: "first range does not start at minimum", ranges: []*RangeAssignment{cover[1], cover[2]}, wantErr: "minimum bound"},
		{name: "gap between ranges", ranges: []*RangeAssignment{cover[0], assignment(mustRange(t, b8(0x60), b8(0x80)), "exec-2"), cover[2]}, wantErr: "not adjacent"},
		{name: "overlap between ranges", ranges: []*RangeAssignment{cover[0], assignment(mustRange(t, b8(0x20), b8(0x80)), "exec-2"), cover[2]}, wantErr: "not adjacent"},
		{name: "duplicate lower bounds", ranges: []*RangeAssignment{cover[0], assignment(mustRange(t, b8(0x00), b8(0x80)), "exec-2"), cover[2]}, wantErr: "not adjacent"},
		{name: "last range is not final", ranges: []*RangeAssignment{cover[0], cover[1]}, wantErr: "end of the key space"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewRangeCover(tt.ranges)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, len(tt.wantOrder), got.Len())
			assert.Equal(t, tt.wantOrder, executorIDs(got.Ranges()))
		})
	}
}

func TestRangeCover_GetRangeForKey(t *testing.T) {
	cover, err := NewRangeCover(validCover(t))
	require.NoError(t, err)

	tests := []struct {
		name      string
		key       []byte
		wantOwner string
	}{
		{name: "minimum bound", key: b8(0x00), wantOwner: "exec-1"},
		{name: "first range upper interior", key: b8(0x3f), wantOwner: "exec-1"},
		{name: "middle range at lower bound", key: b8(0x40), wantOwner: "exec-2"},
		{name: "middle range interior", key: b8(0x60), wantOwner: "exec-2"},
		{name: "final range at lower bound", key: b8(0x80), wantOwner: "exec-3"},
		{name: "final range upper interior", key: b8(0xff), wantOwner: "exec-3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cover.GetRangeForKey(tt.key)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantOwner, got.ExecutorID)
		})
	}
}

func TestRangeCover_Immutability(t *testing.T) {
	base := validCover(t)
	input := []*RangeAssignment{base[2], base[0], base[1]}

	cover, err := NewRangeCover(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"exec-3", "exec-1", "exec-2"}, executorIDs(input))

	ranges := cover.Ranges()
	ranges[0] = nil
	assert.Equal(t, []string{"exec-1", "exec-2", "exec-3"}, executorIDs(cover.Ranges()))
}
