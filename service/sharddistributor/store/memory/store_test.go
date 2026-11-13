package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

const (
	testNamespace = "test-namespace"
	executorA     = "executor-a"
	executorB     = "executor-b"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore(clock.NewMockedTimeSource())
	t.Cleanup(s.Stop)
	return s
}

func guard(held bool) store.GuardFunc {
	return func(txn store.Txn) (store.Txn, error) {
		return txn.(*Txn).If(func() bool { return held }), nil
	}
}

func heartbeatState(status types.ExecutorStatus, metadata map[string]string) store.HeartbeatState {
	return store.HeartbeatState{
		LastHeartbeat: time.Unix(100, 0),
		Status:        status,
		Metadata:      metadata,
		HostMetadata:  &types.HostMetadata{HostName: "host-1"},
	}
}

func assignment(modRevision int64, shardIDs ...string) store.AssignedState {
	shards := make(map[string]*types.ShardAssignment, len(shardIDs))
	for _, shardID := range shardIDs {
		shards[shardID] = &types.ShardAssignment{Status: types.AssignmentStatusREADY}
	}
	return store.AssignedState{AssignedShards: shards, ModRevision: modRevision}
}

func assign(t *testing.T, s *Store, assignments map[string]store.AssignedState) {
	t.Helper()
	request := store.AssignShardsRequest{NewState: &store.NamespaceState{
		AssignmentState: store.AssignmentState{ShardAssignments: assignments},
	}}
	require.NoError(t, s.AssignShards(context.Background(), testNamespace, request, store.NopGuard()))
}

func TestRecordHeartbeat(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	_, err := s.GetExecutorState(ctx, testNamespace, executorA)
	require.ErrorIs(t, err, store.ErrExecutorNotFound)

	require.NoError(t, s.RecordHeartbeat(ctx, testNamespace, executorA, heartbeatState(types.ExecutorStatusACTIVE, map[string]string{"a": "1"})))
	first, err := s.GetAssignmentState(ctx, testNamespace)
	require.NoError(t, err)

	// Same metadata: the assignment version stays put.
	require.NoError(t, s.RecordHeartbeat(ctx, testNamespace, executorA, heartbeatState(types.ExecutorStatusDRAINING, map[string]string{"a": "1"})))
	unchanged, err := s.GetAssignmentState(ctx, testNamespace)
	require.NoError(t, err)
	assert.Equal(t, first.AssignmentRevision, unchanged.AssignmentRevision)
	assert.Greater(t, unchanged.Revision, first.Revision)

	// New metadata is merged into what is stored and moves the version.
	require.NoError(t, s.RecordHeartbeat(ctx, testNamespace, executorA, heartbeatState(types.ExecutorStatusDRAINING, map[string]string{"b": "2"})))
	changed, err := s.GetAssignmentState(ctx, testNamespace)
	require.NoError(t, err)
	assert.Greater(t, changed.AssignmentRevision, unchanged.AssignmentRevision)

	state, err := s.GetExecutorState(ctx, testNamespace, executorA)
	require.NoError(t, err)
	assert.Equal(t, types.ExecutorStatusDRAINING, state.Heartbeat.Status)
	assert.Equal(t, map[string]string{"a": "1", "b": "2"}, state.Heartbeat.Metadata)
	assert.Equal(t, "host-1", state.Heartbeat.Hostname())
	assert.Nil(t, state.Assignment)
}

func TestAssignShards(t *testing.T) {
	invalidGuard := func(store.Txn) (store.Txn, error) { return struct{}{}, nil }

	tests := []struct {
		name string
		// Executor A is assigned at revision 1 before each case runs.
		request     store.AssignShardsRequest
		guard       store.GuardFunc
		wantErr     error
		wantErrText string
		wantShards  map[string][]string
	}{
		{
			name: "writes new assignments and deletes executors",
			request: store.AssignShardsRequest{
				NewState:          newState(map[string]store.AssignedState{executorB: assignment(0, "shard-1", "shard-2")}),
				ExecutorsToDelete: map[string]int64{executorA: 1},
			},
			guard:      store.NopGuard(),
			wantShards: map[string][]string{executorB: {"shard-1", "shard-2"}},
		},
		{
			name: "only writes changed executors",
			request: store.AssignShardsRequest{
				NewState: newState(map[string]store.AssignedState{
					executorA: assignment(1, "shard-ignored"),
					executorB: assignment(0, "shard-2"),
				}),
				ChangedExecutors: map[string]struct{}{executorB: {}},
			},
			guard:      store.NopGuard(),
			wantShards: map[string][]string{executorA: {"shard-1"}, executorB: {"shard-2"}},
		},
		{
			name: "rejects a stale mod revision",
			request: store.AssignShardsRequest{
				NewState:         newState(map[string]store.AssignedState{executorA: assignment(0, "shard-1"), executorB: assignment(0, "shard-2")}),
				ChangedExecutors: map[string]struct{}{executorB: {}},
			},
			guard:      store.NopGuard(),
			wantErr:    store.ErrVersionConflict,
			wantShards: map[string][]string{executorA: {"shard-1"}},
		},
		{
			name: "rejects deleting an executor whose assignment moved",
			request: store.AssignShardsRequest{
				NewState:          newState(nil),
				ExecutorsToDelete: map[string]int64{executorA: 0},
			},
			guard:      store.NopGuard(),
			wantErr:    store.ErrVersionConflict,
			wantShards: map[string][]string{executorA: {"shard-1"}},
		},
		{
			name: "rejects a failed guard",
			request: store.AssignShardsRequest{
				NewState: newState(map[string]store.AssignedState{executorB: assignment(0, "shard-2")}),
			},
			guard:      guard(false),
			wantErr:    store.ErrVersionConflict,
			wantShards: map[string][]string{executorA: {"shard-1"}},
		},
		{
			name: "rejects a guard for another backend",
			request: store.AssignShardsRequest{
				NewState: newState(map[string]store.AssignedState{executorB: assignment(0, "shard-2")}),
			},
			guard:       invalidGuard,
			wantErrText: "invalid transaction type",
			wantShards:  map[string][]string{executorA: {"shard-1"}},
		},
		{
			name: "skips the guard when nothing is written",
			request: store.AssignShardsRequest{
				NewState:         newState(map[string]store.AssignedState{executorA: assignment(1, "shard-1")}),
				ChangedExecutors: map[string]struct{}{},
			},
			guard:      guard(false),
			wantShards: map[string][]string{executorA: {"shard-1"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := newTestStore(t)
			assign(t, s, map[string]store.AssignedState{executorA: assignment(0, "shard-1")})

			err := s.AssignShards(ctx, testNamespace, tt.request, tt.guard)
			switch {
			case tt.wantErr != nil:
				require.ErrorIs(t, err, tt.wantErr)
			case tt.wantErrText != "":
				require.ErrorContains(t, err, tt.wantErrText)
			default:
				require.NoError(t, err)
			}

			state, err := s.GetAssignmentState(ctx, testNamespace)
			require.NoError(t, err)
			gotShards := make(map[string][]string, len(state.ShardAssignments))
			for executorID, assigned := range state.ShardAssignments {
				for shardID := range assigned.AssignedShards {
					gotShards[executorID] = append(gotShards[executorID], shardID)
				}
			}
			for executorID := range tt.wantShards {
				assert.ElementsMatch(t, tt.wantShards[executorID], gotShards[executorID], executorID)
			}
			assert.Len(t, gotShards, len(tt.wantShards))
		})
	}
}

func newState(assignments map[string]store.AssignedState) *store.NamespaceState {
	return &store.NamespaceState{AssignmentState: store.AssignmentState{ShardAssignments: assignments}}
}

func TestAssignShardsStampsRevisions(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	assign(t, s, map[string]store.AssignedState{executorA: assignment(0, "shard-1")})

	state, err := s.GetAssignmentState(ctx, testNamespace)
	require.NoError(t, err)
	assigned := state.ShardAssignments[executorA]
	assert.Equal(t, state.Revision, assigned.ModRevision)
	assert.Equal(t, state.Revision, state.AssignmentRevision)

	// Statistics are only accepted against the current assignment.
	stats := map[string]store.ShardStatistics{"shard-1": {SmoothedLoad: 1}}
	require.ErrorIs(t, s.RecordShardStatistics(ctx, testNamespace, executorA, assigned.ModRevision-1, stats), store.ErrVersionConflict)
	require.NoError(t, s.RecordShardStatistics(ctx, testNamespace, executorA, assigned.ModRevision, stats))

	full, err := s.GetState(ctx, testNamespace)
	require.NoError(t, err)
	assert.Equal(t, stats, full.ShardStats)
}

func TestDeleteShardStats(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	require.NoError(t, s.RecordHeartbeat(ctx, testNamespace, executorA, heartbeatState(types.ExecutorStatusACTIVE, nil)))
	require.NoError(t, s.RecordShardStatisticsBatch(ctx, testNamespace, []store.ExecutorShardStatistics{
		{ExecutorID: executorA, Statistics: map[string]store.ShardStatistics{"shard-1": {}, "shard-2": {}}},
		{ExecutorID: executorB, Statistics: map[string]store.ShardStatistics{"shard-3": {}}},
	}))

	require.Error(t, s.DeleteShardStats(ctx, testNamespace, []string{"shard-1"}, guard(false)))
	require.NoError(t, s.DeleteShardStats(ctx, testNamespace, []string{"shard-1", "shard-3"}, store.NopGuard()))

	state, err := s.GetState(ctx, testNamespace)
	require.NoError(t, err)
	assert.Equal(t, map[string]store.ShardStatistics{"shard-2": {}}, state.ShardStats)
	// Executor B had nothing but statistics, so it is gone with them.
	assert.Contains(t, state.Executors, executorA)
	assert.NotContains(t, state.Executors, executorB)
}

func TestDrainShards(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	require.Error(t, s.DrainShards(ctx, testNamespace, []string{"bad/shard"}))
	require.NoError(t, s.DrainShards(ctx, testNamespace, []string{"shard-2", "shard-1"}))

	drained, err := s.GetDrainedShards(ctx, testNamespace)
	require.NoError(t, err)
	assert.Equal(t, []string{"shard-1", "shard-2"}, drained)

	removed, err := s.UndrainShards(ctx, testNamespace, []string{"shard-1", "shard-3"})
	require.NoError(t, err)
	assert.Equal(t, []string{"shard-1"}, removed)

	state, err := s.GetAssignmentState(ctx, testNamespace)
	require.NoError(t, err)
	assert.Equal(t, map[string]struct{}{"shard-2": {}}, state.DrainedShards)
	assert.Equal(t, state.Revision, state.AssignmentRevision)
}

func TestDrainHosts(t *testing.T) {
	ctx := context.Background()
	timeSource := clock.NewMockedTimeSourceAt(time.Unix(1000, 0))
	s := NewStore(timeSource)
	t.Cleanup(s.Stop)

	require.NoError(t, s.DrainHosts(ctx, testNamespace, []store.DrainedHost{{Hostname: "host-b", Reason: "first"}, {Hostname: "host-a"}}))
	// Draining again keeps the original record.
	timeSource.Advance(time.Hour)
	require.NoError(t, s.DrainHosts(ctx, testNamespace, []store.DrainedHost{{Hostname: "host-b", Reason: "second"}}))

	hosts, err := s.GetDrainedHosts(ctx, testNamespace)
	require.NoError(t, err)
	drainedAt := time.Unix(1000, 0).UTC()
	assert.Equal(t, []store.DrainedHost{
		{Hostname: "host-a", DrainedAt: drainedAt},
		{Hostname: "host-b", DrainedAt: drainedAt, Reason: "first"},
	}, hosts)

	removed, err := s.UndrainHosts(ctx, testNamespace, []string{"host-a", "host-c"})
	require.NoError(t, err)
	assert.Equal(t, []string{"host-a"}, removed)
}

func TestResetNamespace(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	require.NoError(t, s.RecordHeartbeat(ctx, testNamespace, executorA, heartbeatState(types.ExecutorStatusACTIVE, map[string]string{"a": "1"})))
	assign(t, s, map[string]store.AssignedState{executorA: assignment(0, "shard-1")})
	require.NoError(t, s.DrainShards(ctx, testNamespace, []string{"shard-1"}))
	require.NoError(t, s.DrainHosts(ctx, testNamespace, []store.DrainedHost{{Hostname: "host-1"}}))
	before, err := s.GetAssignmentState(ctx, testNamespace)
	require.NoError(t, err)

	removed, err := s.ResetNamespace(ctx, testNamespace)
	require.NoError(t, err)
	// Heartbeat (3), host metadata, metadata key, assignment, drained shard,
	// drained host and the assignment version.
	assert.Equal(t, int64(9), removed)

	state, err := s.GetState(ctx, testNamespace)
	require.NoError(t, err)
	assert.Empty(t, state.Executors)
	assert.Empty(t, state.DrainedShards)
	assert.Empty(t, state.DrainedHosts)
	assert.Greater(t, state.AssignmentRevision, before.AssignmentRevision)
}

func TestReadsReturnCopies(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	require.NoError(t, s.RecordHeartbeat(ctx, testNamespace, executorA, heartbeatState(types.ExecutorStatusACTIVE, map[string]string{"a": "1"})))
	assign(t, s, map[string]store.AssignedState{executorA: assignment(0, "shard-1")})

	state, err := s.GetState(ctx, testNamespace)
	require.NoError(t, err)
	state.ExecutorMetadata[executorA]["a"] = "mutated"
	state.ShardAssignments[executorA].AssignedShards["shard-1"].Status = types.AssignmentStatusINVALID

	reread, err := s.GetState(ctx, testNamespace)
	require.NoError(t, err)
	assert.Equal(t, "1", reread.ExecutorMetadata[executorA]["a"])
	assert.Equal(t, types.AssignmentStatusREADY, reread.ShardAssignments[executorA].AssignedShards["shard-1"].Status)
}

func TestSubscribeToNamespaceChanges(t *testing.T) {
	ctx := context.Background()
	s := NewStore(clock.NewMockedTimeSource())

	changes, err := s.SubscribeToNamespaceChanges(testNamespace)
	require.NoError(t, err)
	requireSignal(t, changes)

	// A heartbeat that changes no metadata is not a namespace change.
	require.NoError(t, s.RecordHeartbeat(ctx, testNamespace, executorA, heartbeatState(types.ExecutorStatusACTIVE, nil)))
	requireNoSignal(t, changes)

	assign(t, s, map[string]store.AssignedState{executorA: assignment(0, "shard-1")})
	requireSignal(t, changes)

	// Rewriting the same assignment is not a change either. The heartbeat took
	// revision 1, so the assignment is at revision 2.
	assign(t, s, map[string]store.AssignedState{executorA: assignment(2, "shard-1")})
	requireNoSignal(t, changes)

	s.Stop()
	_, open := <-changes
	assert.False(t, open)
	_, err = s.SubscribeToNamespaceChanges(testNamespace)
	assert.Error(t, err)
}

func TestSubscribeToExecutorStatusChanges(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := newTestStore(t)

	revisions, err := s.SubscribeToExecutorStatusChanges(ctx, testNamespace)
	require.NoError(t, err)

	require.NoError(t, s.RecordHeartbeat(ctx, testNamespace, executorA, heartbeatState(types.ExecutorStatusACTIVE, nil)))
	require.NoError(t, s.RecordHeartbeat(ctx, testNamespace, executorA, heartbeatState(types.ExecutorStatusACTIVE, nil)))
	require.NoError(t, s.RecordHeartbeat(ctx, testNamespace, executorA, heartbeatState(types.ExecutorStatusDRAINING, nil)))

	// The first and last heartbeats changed the status; only the latest revision is kept.
	state, err := s.GetAssignmentState(ctx, testNamespace)
	require.NoError(t, err)
	assert.Equal(t, state.Revision, <-revisions)
	requireNoSignal(t, revisions)

	cancel()
	require.Eventually(t, func() bool {
		_, open := <-revisions
		return !open
	}, time.Second, time.Millisecond)
}

func requireSignal[T any](t *testing.T, ch <-chan T) {
	t.Helper()
	select {
	case <-ch:
	default:
		require.Fail(t, "expected a signal")
	}
}

func requireNoSignal[T any](t *testing.T, ch <-chan T) {
	t.Helper()
	select {
	case v, open := <-ch:
		require.False(t, open, "unexpected signal %v", v)
	default:
	}
}
