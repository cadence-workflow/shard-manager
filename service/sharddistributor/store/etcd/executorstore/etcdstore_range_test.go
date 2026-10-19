package executorstore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/testhelper"
)

func TestPutRangeAndGetRange(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	s := createStore(t, tc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tests := map[string]*store.RangeAssignment{
		"finite": {
			Range:      mustRange(t, []byte{0x40, 0, 0, 0, 0, 0, 0, 0}, []byte{0x80, 0, 0, 0, 0, 0, 0, 0}),
			ExecutorID: "host-1@uuid-1",
			Status:     types.AssignmentStatusREADY,
		},
		"final": {
			Range:      mustFinalRange(t, []byte{0x80, 0, 0, 0, 0, 0, 0, 0}),
			ExecutorID: "host-2@uuid-2",
			Status:     types.AssignmentStatusREADY,
		},
	}

	for name, assignment := range tests {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, s.PutRange(ctx, tc.Namespace, assignment))

			got, err := s.GetRange(ctx, tc.Namespace, assignment.Range.LowInclusive)
			require.NoError(t, err)
			require.Equal(t, assignment.Range.LowInclusive, got.Range.LowInclusive)
			require.Equal(t, assignment.Range.HighExclusive, got.Range.HighExclusive)
			require.Equal(t, assignment.Range.IsFinalRange(), got.Range.IsFinalRange())
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

	_, err := s.GetRange(ctx, tc.Namespace, []byte{0x40, 0, 0, 0, 0, 0, 0, 0})
	require.ErrorIs(t, err, store.ErrRangeNotFound)
}

func TestDeleteRange(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	s := createStore(t, tc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	low := []byte{0x40, 0, 0, 0, 0, 0, 0, 0}
	assignment := &store.RangeAssignment{
		Range:      mustRange(t, low, []byte{0x80, 0, 0, 0, 0, 0, 0, 0}),
		ExecutorID: "host-1@uuid-1",
		Status:     types.AssignmentStatusREADY,
	}

	require.NoError(t, s.PutRange(ctx, tc.Namespace, assignment))

	require.NoError(t, s.DeleteRange(ctx, tc.Namespace, low))
	_, err := s.GetRange(ctx, tc.Namespace, low)
	require.ErrorIs(t, err, store.ErrRangeNotFound)

	require.NoError(t, s.DeleteRange(ctx, tc.Namespace, low))
}

func TestListRanges_SortedByLowerBound(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	s := createStore(t, tc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Insert out of order so the test verifies etcd returns them sorted.
	assignment := func(low, high []byte, executor string) *store.RangeAssignment {
		return &store.RangeAssignment{
			Range:      mustRange(t, low, high),
			ExecutorID: executor,
			Status:     types.AssignmentStatusREADY,
		}
	}

	high := assignment([]byte{0x80, 0, 0, 0, 0, 0, 0, 0}, []byte{0xff, 0, 0, 0, 0, 0, 0, 0}, "host-3@uuid-3")
	mid1 := assignment([]byte{0x10, 0, 0, 0, 0, 0, 0, 0}, []byte{0x7f, 0, 0, 0, 0, 0, 0, 0}, "host-2@uuid-2")
	low := assignment([]byte{0x00, 0, 0, 0, 0, 0, 0, 0}, []byte{0x40, 0, 0, 0, 0, 0, 0, 0}, "host-1@uuid-1")
	mid2 := assignment([]byte{0x7f, 0, 0, 0, 0, 0, 0, 0}, []byte{0x80, 0, 0, 0, 0, 0, 0, 0}, "host-2@uuid-2")

	for _, a := range []*store.RangeAssignment{high, low, mid1, mid2} {
		require.NoError(t, s.PutRange(ctx, tc.Namespace, a))
	}

	got, err := s.ListRanges(ctx, tc.Namespace)
	require.NoError(t, err)
	require.Len(t, got, 4)

	require.Equal(t, low.ExecutorID, got[0].ExecutorID)
	require.Equal(t, mid1.ExecutorID, got[1].ExecutorID)
	require.Equal(t, mid2.ExecutorID, got[2].ExecutorID)
	require.Equal(t, high.ExecutorID, got[3].ExecutorID)

	// The ranges must be sorted by lower bound.
	require.Equal(t, low.Range.LowInclusive, got[0].Range.LowInclusive)
	require.Equal(t, mid1.Range.LowInclusive, got[1].Range.LowInclusive)
	require.Equal(t, mid2.Range.LowInclusive, got[2].Range.LowInclusive)
	require.Equal(t, high.Range.LowInclusive, got[3].Range.LowInclusive)

	// for i := 1; i < len(got); i++ {
	// 	require.True(t, got[i-1].Range.Compare(got[i].Range) < 0, "ranges must be sorted by lower bound")
	// }
}

func TestPutRange_RejectsInvalidBoundWidth(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	s := createStore(t, tc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tests := map[string]*types.Range{
		"empty low bound":        rawRange([]byte{}, []byte{0x40, 0, 0, 0, 0, 0, 0, 0}),
		"short low bound":        rawRange([]byte{0x40}, []byte{0x40, 0, 0, 0, 0, 0, 0, 0}),
		"long low bound":         rawRange([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0}, []byte{0x40, 0, 0, 0, 0, 0, 0, 0}),
		"empty finite high":      rawRange([]byte{0x00, 0, 0, 0, 0, 0, 0, 0}, []byte{}),
		"short finite high":      rawRange([]byte{0x00, 0, 0, 0, 0, 0, 0, 0}, []byte{0x40}),
		"long non-sentinel high": rawRange([]byte{0x00, 0, 0, 0, 0, 0, 0, 0}, []byte{0x40, 0, 0, 0, 0, 0, 0, 0, 0}),
	}

	for name, rng := range tests {
		t.Run(name, func(t *testing.T) {
			err := s.PutRange(ctx, tc.Namespace, &store.RangeAssignment{
				Range:      rng,
				ExecutorID: "host-1@uuid-1",
				Status:     types.AssignmentStatusREADY,
			})
			require.Error(t, err)
		})
	}
}

func TestListRangesAndGetRange_PopulateModRevision(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	s := createStore(t, tc)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	low := []byte{0x00, 0, 0, 0, 0, 0, 0, 0}
	require.NoError(t, s.PutRange(ctx, tc.Namespace, &store.RangeAssignment{
		Range:      mustRange(t, low, []byte{0x40, 0, 0, 0, 0, 0, 0, 0}),
		ExecutorID: "host-1@uuid-1",
		Status:     types.AssignmentStatusREADY,
	}))

	got, err := s.GetRange(ctx, tc.Namespace, low)
	require.NoError(t, err)
	require.NotZero(t, got.ModRevision)

	listed, err := s.ListRanges(ctx, tc.Namespace)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, got.ModRevision, listed[0].ModRevision)
}

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

// rawRange builds a Range without the NewRange validity check, so bound-width
// cases that types.NewRange would reject for other reasons can be exercised
// directly against the store's width validation.
func rawRange(low, high []byte) *types.Range {
	return &types.Range{LowInclusive: low, HighExclusive: high}
}
