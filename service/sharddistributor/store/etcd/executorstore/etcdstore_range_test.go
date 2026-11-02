package executorstore

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/testhelper"
)

const (
	testRangeID      = "550e8400-e29b-41d4-a716-446655440000"
	testFinalRangeID = "123e4567-e89b-12d3-a456-426614174000"
)

func TestPutRangeAndGetRange(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	s := createStore(t, tc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tests := map[string]*store.RangeAssignment{
		"finite": {
			ID:         testRangeID,
			Range:      mustRange(t, []byte{0x40, 0, 0, 0, 0, 0, 0, 0}, []byte{0x80, 0, 0, 0, 0, 0, 0, 0}),
			ExecutorID: "host-1@uuid-1",
			Status:     types.AssignmentStatusREADY,
		},
		"final": {
			ID:         testFinalRangeID,
			Range:      mustFinalRange(t, []byte{0x80, 0, 0, 0, 0, 0, 0, 0}),
			ExecutorID: "host-2@uuid-2",
			Status:     types.AssignmentStatusREADY,
		},
		// Non-UUID range IDs are accepted and preserved.
		"non-uuid id": {
			ID:         "range-1",
			Range:      mustRange(t, []byte{0x00, 0, 0, 0, 0, 0, 0, 0}, []byte{0x40, 0, 0, 0, 0, 0, 0, 0}),
			ExecutorID: "host-3@uuid-3",
			Status:     types.AssignmentStatusREADY,
		},
		// Bounds are not required to be fingerprint-width. Only their relative
		// ordering matters.
		"non-fingerprint width": {
			ID:         "range-2",
			Range:      mustRange(t, []byte{0x00}, []byte{0x00, 0x10}),
			ExecutorID: "host-4@uuid-4",
			Status:     types.AssignmentStatusREADY,
		},
	}

	for name, assignment := range tests {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, s.PutRange(ctx, tc.Namespace, assignment))

			got, err := s.GetRange(ctx, tc.Namespace, assignment.ID)
			require.NoError(t, err)
			require.Equal(t, assignment.ID, got.ID)
			require.Equal(t, assignment.Range, got.Range)
			require.Equal(t, assignment.ExecutorID, got.ExecutorID)
			require.Equal(t, assignment.Status, got.Status)
		})
	}
}

func TestGetRange_NotFound(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	s := createStore(t, tc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := s.GetRange(ctx, tc.Namespace, testRangeID)
	require.ErrorIs(t, err, store.ErrRangeNotFound)
}

func TestDeleteRange(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	s := createStore(t, tc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rangeID := testRangeID
	assignment := &store.RangeAssignment{
		ID:         rangeID,
		Range:      mustRange(t, []byte{0x40, 0, 0, 0, 0, 0, 0, 0}, []byte{0x80, 0, 0, 0, 0, 0, 0, 0}),
		ExecutorID: "host-1@uuid-1",
		Status:     types.AssignmentStatusREADY,
	}

	require.NoError(t, s.PutRange(ctx, tc.Namespace, assignment))

	require.NoError(t, s.DeleteRange(ctx, tc.Namespace, rangeID))
	_, err := s.GetRange(ctx, tc.Namespace, rangeID)
	require.ErrorIs(t, err, store.ErrRangeNotFound)

	require.NoError(t, s.DeleteRange(ctx, tc.Namespace, rangeID))
}

// TestGetAndDeleteRange_RejectInvalidID verifies the ID validation performed
// by both operations before building an etcd key. Without it, an ID
// containing '/' would create a key deeper in the ranges keyspace that
// ListRanges cannot parse.
func TestGetAndDeleteRange_RejectInvalidID(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	s := createStore(t, tc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tests := []struct {
		name    string
		rangeID string
		wantErr string
	}{
		{name: "empty", rangeID: "", wantErr: "must not be empty"},
		{name: "with slash", rangeID: "bad/id", wantErr: "must not contain '/'"},
	}

	for _, tt := range tests {
		t.Run("get "+tt.name, func(t *testing.T) {
			_, err := s.GetRange(ctx, tc.Namespace, tt.rangeID)
			require.ErrorContains(t, err, tt.wantErr)
		})
		t.Run("delete "+tt.name, func(t *testing.T) {
			require.ErrorContains(t, s.DeleteRange(ctx, tc.Namespace, tt.rangeID), tt.wantErr)
		})
	}
}

func TestListRanges_SortedByLowerBound(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	s := createStore(t, tc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	empty, err := s.ListRanges(ctx, tc.Namespace)
	require.NoError(t, err)
	require.NotNil(t, empty)
	require.Empty(t, empty)

	assignment := func(id string, low, high []byte, executor string) *store.RangeAssignment {
		return &store.RangeAssignment{
			ID:         id,
			Range:      mustRange(t, low, high),
			ExecutorID: executor,
			Status:     types.AssignmentStatusREADY,
		}
	}

	// IDs are intentionally ordered opposite to LowInclusive so the test proves
	// ListRanges sorts by LowInclusive rather than by etcd key order.
	high := assignment("00000000-0000-4000-8000-000000000001", []byte{0x80, 0, 0, 0, 0, 0, 0, 0}, []byte{0xff, 0, 0, 0, 0, 0, 0, 0}, "host-3@uuid-3")
	mid1 := assignment("00000000-0000-4000-8000-000000000003", []byte{0x10, 0, 0, 0, 0, 0, 0, 0}, []byte{0x7f, 0, 0, 0, 0, 0, 0, 0}, "host-2@uuid-2")
	low := assignment("00000000-0000-4000-8000-000000000004", []byte{0x00, 0, 0, 0, 0, 0, 0, 0}, []byte{0x40, 0, 0, 0, 0, 0, 0, 0}, "host-1@uuid-1")
	mid2 := assignment("00000000-0000-4000-8000-000000000002", []byte{0x7f, 0, 0, 0, 0, 0, 0, 0}, []byte{0x80, 0, 0, 0, 0, 0, 0, 0}, "host-2@uuid-2")

	for _, a := range []*store.RangeAssignment{high, low, mid1, mid2} {
		require.NoError(t, s.PutRange(ctx, tc.Namespace, a))
	}

	got, err := s.ListRanges(ctx, tc.Namespace)
	require.NoError(t, err)
	require.Len(t, got, 4)

	require.Equal(t, low.ID, got[0].ID)
	require.Equal(t, mid1.ID, got[1].ID)
	require.Equal(t, mid2.ID, got[2].ID)
	require.Equal(t, high.ID, got[3].ID)

	for i := 1; i < len(got); i++ {
		require.Negative(t, bytes.Compare(got[i-1].Range.LowInclusive, got[i].Range.LowInclusive),
			"ranges must be sorted by lower bound")
	}
}

// TestSortRangeAssignments verifies the ID tie-break. An etcd-backed test
// would not cover this case because etcd already orders equal-bound entries
// by key.
func TestSortRangeAssignments(t *testing.T) {
	rangeAt := func(low byte) types.Range {
		return mustRange(t, []byte{low}, []byte{low + 1})
	}

	assignments := []*store.RangeAssignment{
		{ID: "range-b", Range: rangeAt(0x10)},
		{ID: "range-a", Range: rangeAt(0x10)},
		{ID: "range-c", Range: rangeAt(0x20)},
	}

	slices.SortFunc(assignments, sortRangeAssignments)

	ids := make([]string, 0, len(assignments))
	for _, a := range assignments {
		ids = append(ids, a.ID)
	}
	require.Equal(t, []string{"range-a", "range-b", "range-c"}, ids)
}

func TestValidateRangeAssignment(t *testing.T) {
	validRange := mustRange(t, []byte{0x00, 0, 0, 0, 0, 0, 0, 0}, []byte{0x40, 0, 0, 0, 0, 0, 0, 0})
	equalBounds := rawRange([]byte{0x40, 0, 0, 0, 0, 0, 0, 0}, []byte{0x40, 0, 0, 0, 0, 0, 0, 0})
	invertedBounds := rawRange([]byte{0x80, 0, 0, 0, 0, 0, 0, 0}, []byte{0x40, 0, 0, 0, 0, 0, 0, 0})

	tests := []struct {
		name       string
		assignment *store.RangeAssignment
		wantErr    string
	}{
		{name: "nil assignment", assignment: nil, wantErr: "must not be nil"},
		{name: "empty ID", assignment: &store.RangeAssignment{Range: validRange}, wantErr: "range ID must not be empty"},
		{name: "ID with slash", assignment: &store.RangeAssignment{ID: "bad/id", Range: validRange}, wantErr: "must not contain '/'"},
		{name: "zero-value range", assignment: &store.RangeAssignment{ID: testRangeID, Range: types.Range{}}, wantErr: "range is invalid"},
		{name: "equal bounds", assignment: &store.RangeAssignment{ID: testRangeID, Range: equalBounds}, wantErr: "range is invalid"},
		{name: "inverted bounds", assignment: &store.RangeAssignment{ID: testRangeID, Range: invertedBounds}, wantErr: "range is invalid"},
		{name: "valid", assignment: &store.RangeAssignment{ID: testRangeID, Range: validRange}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRangeAssignment(tt.assignment)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestPutRange_RejectsInvalidAssignment(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	s := createStore(t, tc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := s.PutRange(ctx, tc.Namespace, &store.RangeAssignment{
		ID:     testRangeID,
		Range:  types.Range{},
		Status: types.AssignmentStatusREADY,
	})
	require.Error(t, err)
}

func TestListRangesAndGetRange_PopulateModRevision(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	s := createStore(t, tc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rangeID := testRangeID
	require.NoError(t, s.PutRange(ctx, tc.Namespace, &store.RangeAssignment{
		ID:         rangeID,
		Range:      mustRange(t, []byte{0x00, 0, 0, 0, 0, 0, 0, 0}, []byte{0x40, 0, 0, 0, 0, 0, 0, 0}),
		ExecutorID: "host-1@uuid-1",
		Status:     types.AssignmentStatusREADY,
	}))

	got, err := s.GetRange(ctx, tc.Namespace, rangeID)
	require.NoError(t, err)
	require.NotZero(t, got.ModRevision)

	listed, err := s.ListRanges(ctx, tc.Namespace)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, got.ModRevision, listed[0].ModRevision)
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

// rawRange builds a Range without the constructor's validation, for the invalid
// shapes validateRangeAssignment must reject.
func rawRange(low, high []byte) types.Range {
	return types.Range{LowInclusive: low, HighExclusive: high}
}
