package executorstore

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdkeys"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdtypes"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/testhelper"
)

// putRangeFixture writes a range directly to etcd and records its ModRevision.
// Tests seed etcd directly because AssignRanges is not implemented yet.
func putRangeFixture(t *testing.T, tc *testhelper.StoreTestCluster, assignment *store.RangeAssignment) {
	t.Helper()
	payload, err := json.Marshal(etcdtypes.FromRangeAssignment(assignment))
	require.NoError(t, err)
	resp, err := tc.Client.Put(context.Background(), etcdkeys.BuildRangeKey(tc.EtcdPrefix, tc.Namespace, assignment.ID), string(payload))
	require.NoError(t, err)
	assignment.ModRevision = resp.Header.Revision
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

func TestListRanges(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	s := createStore(t, tc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	empty, err := s.ListRanges(ctx, tc.Namespace)
	require.NoError(t, err)
	require.NotNil(t, empty)
	require.Empty(t, empty)

	// IDs order opposite to their bounds, so the expected order cannot come from
	// etcd key order.
	high := &store.RangeAssignment{
		ID:    "range-high",
		Range: mustRange(t, []byte{0x80, 0, 0, 0, 0, 0, 0, 0}, []byte{0xc0, 0, 0, 0, 0, 0, 0, 0}),
	}
	final := &store.RangeAssignment{
		ID:    "range-final",
		Range: mustFinalRange(t, []byte{0xc0, 0, 0, 0, 0, 0, 0, 0}),
	}
	low := &store.RangeAssignment{
		ID:    "range-low",
		Range: mustRange(t, []byte{0x00, 0, 0, 0, 0, 0, 0, 0}, []byte{0x80, 0, 0, 0, 0, 0, 0, 0}),
	}

	for _, a := range []*store.RangeAssignment{low, high, final} {
		a.ExecutorID = "host-1@uuid-1"
		a.Status = types.AssignmentStatusREADY
	}
	putRangeFixture(t, tc, low)
	putRangeFixture(t, tc, high)
	putRangeFixture(t, tc, final)

	got, err := s.ListRanges(ctx, tc.Namespace)
	require.NoError(t, err)
	require.Equal(t, []*store.RangeAssignment{low, high, final}, got)
}

func TestListRanges_RejectsKeyWithSlashInID(t *testing.T) {
	tc := testhelper.SetupStoreTestCluster(t)
	s := createStore(t, tc)

	putRangeFixture(t, tc, &store.RangeAssignment{
		ID:         "bad/id",
		Range:      mustFinalRange(t, []byte{0x00, 0, 0, 0, 0, 0, 0, 0}),
		ExecutorID: "host-1@uuid-1",
	})

	_, err := s.ListRanges(context.Background(), tc.Namespace)
	require.ErrorContains(t, err, "must not contain '/'")
}
