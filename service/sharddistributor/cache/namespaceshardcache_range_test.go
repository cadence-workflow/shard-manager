package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

const testRangeNamespace = "range-namespace"

func testBound(first byte) []byte {
	return []byte{first, 0, 0, 0, 0, 0, 0, 0}
}

// testCover is the smallest valid cover: one range over the whole key space.
func testCover(t *testing.T, owner string) []*store.RangeAssignment {
	t.Helper()
	whole, err := types.NewFinalRange(testBound(0x00))
	require.NoError(t, err)
	return []*store.RangeAssignment{{Range: whole, ExecutorID: owner, Status: types.AssignmentStatusREADY}}
}

// testInvalidCover leaves a gap below the first range.
func testInvalidCover(t *testing.T) []*store.RangeAssignment {
	t.Helper()

	final, err := types.NewFinalRange(testBound(0x80))
	require.NoError(t, err)
	return []*store.RangeAssignment{{Range: final, ExecutorID: "executor-invalid"}}
}

func TestShardCache_RangeCoverFollowsNamespaceState(t *testing.T) {
	for _, tt := range []struct {
		name string
		// initial is applied at revision 1 when set.
		initial    []*store.RangeAssignment
		next       []*store.RangeAssignment
		nextRev    int64
		wantOwner  string // empty means ErrRangeCoverNotReady
		wantMetric bool
	}{
		{"no cover written yet", nil, []*store.RangeAssignment{}, 2, "", false},
		{"invalid first cover fails closed", nil, testInvalidCover(t), 2, "", true},
		{"newer cover replaces the previous one", testCover(t, "old"), testCover(t, "new"), 2, "new", false},
		{"invalid cover keeps the previous one", testCover(t, "old"), testInvalidCover(t), 2, "old", true},
		{"cover disappearing keeps the previous one", testCover(t, "old"), []*store.RangeAssignment{}, 2, "old", true},
		{"stale read cannot replace a newer cover", testCover(t, "old"), testCover(t, "stale"), 1, "old", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			metricsClient := metrics.NewMockClient(ctrl)
			if tt.wantMetric {
				scope := metrics.NewMockScope(ctrl)
				metricsClient.EXPECT().Scope(metrics.ShardDistributorRangeCoverScope).Return(scope)
				scope.EXPECT().Tagged(metrics.NamespaceTag(testRangeNamespace)).Return(scope)
				scope.EXPECT().IncCounter(metrics.ShardDistributorRangeCoverInvalid)
			}

			c := newShardCache(testRangeNamespace, nil, testlogger.New(t), clock.NewMockedTimeSource(), metricsClient)
			if tt.initial != nil {
				c.applyNamespaceState(&store.AssignmentState{Revision: 1, Ranges: tt.initial})
			}
			c.applyNamespaceState(&store.AssignmentState{Revision: tt.nextRev, Ranges: tt.next})

			assignment, err := c.GetRangeForShardKey(context.Background(), testBound(0x00))
			if tt.wantOwner == "" {
				require.ErrorIs(t, err, ErrRangeCoverNotReady)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOwner, assignment.ExecutorID)
		})
	}
}

func TestShardCache_GetRangeForShardKey_ReadsStoreOnlyWhenCold(t *testing.T) {
	for _, tt := range []struct {
		name      string
		loaded    bool
		state     *store.AssignmentState
		err       error
		wantOwner string // empty means ErrRangeCoverNotReady
	}{
		{"cold cache loads the cover", false, &store.AssignmentState{Revision: 1, Ranges: testCover(t, "executor-1")}, nil, "executor-1"},
		{"cold cache fails closed when the read fails", false, nil, errors.New("etcd unavailable"), ""},
		{"loaded namespace without a cover answers without a read", true, nil, nil, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			executorStore := store.NewMockStore(ctrl)
			c := newTestShardCache(t, testRangeNamespace, executorStore)
			if tt.loaded {
				c.applyNamespaceState(&store.AssignmentState{Revision: 1})
			} else {
				executorStore.EXPECT().GetAssignmentState(gomock.Any(), testRangeNamespace).Return(tt.state, tt.err)
			}

			// gomock fails an unexpected call inside the refresh goroutine, which would leave the lookup waiting forever.
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			assignment, err := c.GetRangeForShardKey(ctx, testBound(0x00))
			if tt.wantOwner == "" {
				require.ErrorIs(t, err, ErrRangeCoverNotReady)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOwner, assignment.ExecutorID)
		})
	}
}
