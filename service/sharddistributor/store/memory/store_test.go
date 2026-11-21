package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/storetest"
)

const (
	testNamespace = "test-namespace"
	executorA     = "executor-a"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore(clock.NewMockedTimeSource())
	t.Cleanup(s.Stop)
	return s
}

func TestStoreSuite(t *testing.T) {
	storetest.Run(t, storetest.Backend{
		NewStore: func(t *testing.T) store.Store { return newTestStore(t) },
	})
}

func TestGuardsAreIgnored(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	failingGuard := func(store.Txn) (store.Txn, error) { return nil, errors.New("guard must not be applied") }
	require.NoError(t, s.RecordHeartbeat(ctx, testNamespace, executorA, store.HeartbeatState{Status: types.ExecutorStatusACTIVE}))

	require.NoError(t, s.DeleteExecutors(ctx, testNamespace, []string{executorA}, failingGuard))
	_, err := s.GetExecutorState(ctx, testNamespace, executorA)
	require.ErrorIs(t, err, store.ErrExecutorNotFound)
}

func TestReadsReturnCopies(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	require.NoError(t, s.RecordHeartbeat(ctx, testNamespace, executorA, store.HeartbeatState{
		Status:   types.ExecutorStatusACTIVE,
		Metadata: map[string]string{"a": "1"},
	}))
	require.NoError(t, s.AssignShards(ctx, testNamespace, store.AssignShardsRequest{NewState: &store.NamespaceState{
		AssignmentState: store.AssignmentState{ShardAssignments: map[string]store.AssignedState{
			executorA: {AssignedShards: map[string]*types.ShardAssignment{"shard-1": {Status: types.AssignmentStatusREADY}}},
		}},
	}}, store.NopGuard()))

	state, err := s.GetState(ctx, testNamespace)
	require.NoError(t, err)
	state.ExecutorMetadata[executorA]["a"] = "mutated"
	state.ShardAssignments[executorA].AssignedShards["shard-1"].Status = types.AssignmentStatusINVALID

	reread, err := s.GetState(ctx, testNamespace)
	require.NoError(t, err)
	assert.Equal(t, "1", reread.ExecutorMetadata[executorA]["a"])
	assert.Equal(t, types.AssignmentStatusREADY, reread.ShardAssignments[executorA].AssignedShards["shard-1"].Status)
}

func TestStatusSubscriptionKeepsLatestRevision(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	revisions, err := s.SubscribeToExecutorStatusChanges(ctx, testNamespace)
	require.NoError(t, err)

	for _, status := range []types.ExecutorStatus{types.ExecutorStatusACTIVE, types.ExecutorStatusDRAINING} {
		require.NoError(t, s.RecordHeartbeat(ctx, testNamespace, executorA, store.HeartbeatState{Status: status}))
	}

	state, err := s.GetAssignmentState(ctx, testNamespace)
	require.NoError(t, err)
	assert.Equal(t, state.Revision, <-revisions)
	select {
	case revision := <-revisions:
		require.Failf(t, "unexpected signal", "got revision %d", revision)
	default:
	}
}

func TestStop(t *testing.T) {
	s := NewStore(clock.NewMockedTimeSource())
	changes, err := s.SubscribeToNamespaceChanges(testNamespace)
	require.NoError(t, err)
	revisions, err := s.SubscribeToExecutorStatusChanges(context.Background(), testNamespace)
	require.NoError(t, err)

	s.Stop()
	s.Stop()

	<-changes // the signal sent on subscribe
	_, open := <-changes
	assert.False(t, open)
	require.Eventually(t, func() bool {
		_, open := <-revisions
		return !open
	}, time.Second, time.Millisecond)

	_, err = s.SubscribeToNamespaceChanges(testNamespace)
	assert.Error(t, err)
	_, err = s.SubscribeToExecutorStatusChanges(context.Background(), testNamespace)
	assert.Error(t, err)
}
