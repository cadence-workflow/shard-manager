// Package storetest holds the behaviour every store.Store implementation must share.
// Each backend runs the same scenarios, so the implementations cannot drift apart.
package storetest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

const (
	namespace = "storetest"
	executorA = "executor-a"
	executorB = "executor-b"

	// Watch-based backends deliver signals asynchronously, so a signal is awaited for up
	// to signalTimeout, and its absence is only trusted after quietPeriod.
	signalTimeout = 5 * time.Second
	quietPeriod   = 200 * time.Millisecond
)

// Backend is the store implementation a suite run exercises.
type Backend struct {
	// NewStore returns a store holding no state for any namespace. Every scenario gets its own.
	NewStore func(t *testing.T) store.Store
	// FailingGuard returns a guard whose precondition never holds, standing in for a
	// leader that has lost its election. Leave it nil for a backend that does not apply
	// guards, and the scenarios that need one are skipped.
	FailingGuard func() store.GuardFunc
}

// requireFailingGuard returns the backend's failing guard, skipping the test if it has none.
func requireFailingGuard(t *testing.T, b Backend) store.GuardFunc {
	t.Helper()
	if b.FailingGuard == nil {
		t.Skip("backend does not apply leadership guards")
	}
	return b.FailingGuard()
}

// Run runs every scenario against the backend.
func Run(t *testing.T, backend Backend) {
	scenarios := []struct {
		name string
		run  func(*testing.T, Backend)
	}{
		{"RecordHeartbeat", testRecordHeartbeat},
		{"AssignmentRevision", testAssignmentRevision},
		{"AssignShards", testAssignShards},
		{"RecordShardStatistics", testRecordShardStatistics},
		{"RecordShardStatisticsBatch", testRecordShardStatisticsBatch},
		{"DeleteExecutors", testDeleteExecutors},
		{"DeleteShardStats", testDeleteShardStats},
		{"GuardedWrites", testGuardedWrites},
		{"DrainShards", testDrainShards},
		{"DrainHosts", testDrainHosts},
		{"ResetNamespace", testResetNamespace},
		{"Ranges", testRanges},
		{"SubscribeToNamespaceChanges", testSubscribeToNamespaceChanges},
		{"SubscribeToExecutorStatusChanges", testSubscribeToExecutorStatusChanges},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			scenario.run(t, backend)
		})
	}
}

func testRecordHeartbeat(t *testing.T, b Backend) {
	ctx := t.Context()
	s := b.NewStore(t)

	_, err := s.GetExecutorState(ctx, namespace, executorA)
	require.ErrorIs(t, err, store.ErrExecutorNotFound)

	first := heartbeat(types.ExecutorStatusACTIVE, map[string]string{"a": "1"})
	first.ReportedShards = map[string]*types.ShardStatusReport{"shard-1": {Status: types.ShardStatusREADY, ShardLoad: 0.5}}
	require.NoError(t, s.RecordHeartbeat(ctx, namespace, executorA, first))

	state, err := s.GetExecutorState(ctx, namespace, executorA)
	require.NoError(t, err)
	assert.True(t, first.LastHeartbeat.Equal(state.Heartbeat.LastHeartbeat))
	assert.Equal(t, first.ReportedShards, state.Heartbeat.ReportedShards)
	assert.Nil(t, state.Assignment)

	// Metadata is merged, and host metadata is kept when a heartbeat omits it.
	second := heartbeat(types.ExecutorStatusDRAINING, map[string]string{"b": "2"})
	second.HostMetadata = nil
	require.NoError(t, s.RecordHeartbeat(ctx, namespace, executorA, second))

	state, err = s.GetExecutorState(ctx, namespace, executorA)
	require.NoError(t, err)
	assert.Equal(t, types.ExecutorStatusDRAINING, state.Heartbeat.Status)
	assert.Equal(t, map[string]string{"a": "1", "b": "2"}, state.Heartbeat.Metadata)
	assert.Equal(t, "host-1", state.Heartbeat.Hostname())
	assert.Empty(t, state.Heartbeat.ReportedShards)
}

func testAssignmentRevision(t *testing.T, b Backend) {
	ctx := t.Context()
	s := b.NewStore(t)

	recordHeartbeat := func(metadata map[string]string) func() error {
		return func() error {
			return s.RecordHeartbeat(ctx, namespace, executorA, heartbeat(types.ExecutorStatusACTIVE, metadata))
		}
	}
	// Steps run in order against one namespace, each comparing with the previous revision.
	steps := []struct {
		name  string
		write func() error
		moves bool
	}{
		{
			name: "assign shards",
			write: func() error {
				return s.AssignShards(ctx, namespace, assignRequest(map[string]store.AssignedState{executorA: assignment(0, "shard-1")}), store.NopGuard())
			},
			moves: true,
		},
		{name: "heartbeat without metadata", write: recordHeartbeat(nil), moves: false},
		{name: "heartbeat with new metadata", write: recordHeartbeat(map[string]string{"addr": "a"}), moves: true},
		{name: "heartbeat with unchanged metadata", write: recordHeartbeat(map[string]string{"addr": "a"}), moves: false},
		{name: "heartbeat with changed metadata", write: recordHeartbeat(map[string]string{"addr": "b"}), moves: true},
		{name: "drain shard", write: func() error { return s.DrainShards(ctx, namespace, []string{"shard-2"}) }, moves: true},
		{
			name: "undrain shard",
			write: func() error {
				_, err := s.UndrainShards(ctx, namespace, []string{"shard-2"})
				return err
			},
			moves: true,
		},
		{
			name: "drain host",
			write: func() error {
				return s.DrainHosts(ctx, namespace, []store.DrainedHost{{Hostname: "host-1"}})
			},
			moves: false,
		},
		{
			name:  "delete executor",
			write: func() error { return s.DeleteExecutors(ctx, namespace, []string{executorA}, store.NopGuard()) },
			moves: true,
		},
		{
			name: "reset namespace",
			write: func() error {
				_, err := s.ResetNamespace(ctx, namespace)
				return err
			},
			moves: true,
		},
	}

	var previous int64
	for _, step := range steps {
		require.NoError(t, step.write(), step.name)
		state, err := s.GetAssignmentState(ctx, namespace)
		require.NoError(t, err, step.name)
		assert.Equal(t, step.moves, state.AssignmentRevision > previous, step.name)
		previous = state.AssignmentRevision
	}
}

func testAssignShards(t *testing.T, b Backend) {
	tests := []struct {
		name string
		// request builds the request from the mod revision of executor A, which owns
		// shard-1 before each case runs.
		request    func(revision int64) store.AssignShardsRequest
		failGuard  bool
		wantErr    error
		wantShards map[string][]string
	}{
		{
			name: "writes new assignments and deletes executors",
			request: func(revision int64) store.AssignShardsRequest {
				request := assignRequest(map[string]store.AssignedState{executorB: assignment(0, "shard-1", "shard-2")})
				request.ExecutorsToDelete = map[string]int64{executorA: revision}
				return request
			},
			wantShards: map[string][]string{executorB: {"shard-1", "shard-2"}},
		},
		{
			name: "only writes changed executors",
			request: func(revision int64) store.AssignShardsRequest {
				request := assignRequest(map[string]store.AssignedState{
					executorA: assignment(revision, "shard-ignored"),
					executorB: assignment(0, "shard-2"),
				})
				request.ChangedExecutors = map[string]struct{}{executorB: {}}
				return request
			},
			wantShards: map[string][]string{executorA: {"shard-1"}, executorB: {"shard-2"}},
		},
		{
			name: "rejects a stale mod revision of an unchanged executor",
			request: func(revision int64) store.AssignShardsRequest {
				request := assignRequest(map[string]store.AssignedState{
					executorA: assignment(revision-1, "shard-1"),
					executorB: assignment(0, "shard-2"),
				})
				request.ChangedExecutors = map[string]struct{}{executorB: {}}
				return request
			},
			wantErr:    store.ErrVersionConflict,
			wantShards: map[string][]string{executorA: {"shard-1"}},
		},
		{
			name: "rejects deleting an executor whose assignment moved",
			request: func(revision int64) store.AssignShardsRequest {
				request := assignRequest(nil)
				request.ExecutorsToDelete = map[string]int64{executorA: revision - 1}
				return request
			},
			wantErr:    store.ErrVersionConflict,
			wantShards: map[string][]string{executorA: {"shard-1"}},
		},
		{
			name: "rejects a failed guard",
			request: func(int64) store.AssignShardsRequest {
				return assignRequest(map[string]store.AssignedState{executorB: assignment(0, "shard-2")})
			},
			failGuard:  true,
			wantErr:    store.ErrVersionConflict,
			wantShards: map[string][]string{executorA: {"shard-1"}},
		},
		{
			name: "skips the guard when nothing is written",
			request: func(revision int64) store.AssignShardsRequest {
				request := assignRequest(map[string]store.AssignedState{executorA: assignment(revision, "shard-1")})
				request.ChangedExecutors = map[string]struct{}{}
				return request
			},
			failGuard:  true,
			wantShards: map[string][]string{executorA: {"shard-1"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			s := b.NewStore(t)
			require.NoError(t, s.AssignShards(ctx, namespace, assignRequest(map[string]store.AssignedState{executorA: assignment(0, "shard-1")}), store.NopGuard()))
			revision := assignedRevision(t, s, executorA)

			guard := store.NopGuard()
			if tt.failGuard {
				guard = requireFailingGuard(t, b)
			}
			err := s.AssignShards(ctx, namespace, tt.request(revision), guard)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			state, err := s.GetAssignmentState(ctx, namespace)
			require.NoError(t, err)
			gotShards := make(map[string][]string, len(state.ShardAssignments))
			for executorID, assigned := range state.ShardAssignments {
				for shardID := range assigned.AssignedShards {
					gotShards[executorID] = append(gotShards[executorID], shardID)
				}
			}
			assert.Len(t, gotShards, len(tt.wantShards))
			for executorID, want := range tt.wantShards {
				assert.ElementsMatch(t, want, gotShards[executorID], executorID)
			}
		})
	}
}

func testRecordShardStatistics(t *testing.T, b Backend) {
	ctx := t.Context()
	s := b.NewStore(t)
	require.NoError(t, s.AssignShards(ctx, namespace, assignRequest(map[string]store.AssignedState{executorA: assignment(0, "shard-1")}), store.NopGuard()))

	state, err := s.GetAssignmentState(ctx, namespace)
	require.NoError(t, err)
	revision := state.ShardAssignments[executorA].ModRevision
	assert.Equal(t, state.AssignmentRevision, revision, "the assignment and its version are written together")
	assert.GreaterOrEqual(t, state.Revision, revision)

	// Statistics are only accepted against the current assignment.
	stats := map[string]store.ShardStatistics{"shard-1": {SmoothedLoad: 1}}
	require.ErrorIs(t, s.RecordShardStatistics(ctx, namespace, executorA, revision-1, stats), store.ErrVersionConflict)
	require.NoError(t, s.RecordShardStatistics(ctx, namespace, executorA, revision, stats))

	executor, err := s.GetExecutorState(ctx, namespace, executorA)
	require.NoError(t, err)
	assert.Equal(t, 1.0, executor.Statistics["shard-1"].SmoothedLoad)
}

func testRecordShardStatisticsBatch(t *testing.T, b Backend) {
	ctx := t.Context()
	s := b.NewStore(t)
	require.NoError(t, s.RecordHeartbeat(ctx, namespace, executorA, heartbeat(types.ExecutorStatusACTIVE, nil)))

	require.NoError(t, s.RecordShardStatisticsBatch(ctx, namespace, []store.ExecutorShardStatistics{
		{ExecutorID: executorA, Statistics: map[string]store.ShardStatistics{"shard-1": {SmoothedLoad: 1}}},
		{ExecutorID: executorB, Statistics: map[string]store.ShardStatistics{"shard-2": {SmoothedLoad: 2}}},
	}))
	assert.Equal(t, map[string]float64{"shard-1": 1, "shard-2": 2}, smoothedLoads(t, s))

	// An empty update removes the executor's statistics.
	require.NoError(t, s.RecordShardStatisticsBatch(ctx, namespace, []store.ExecutorShardStatistics{
		{ExecutorID: executorA, Statistics: nil},
	}))
	assert.Equal(t, map[string]float64{"shard-2": 2}, smoothedLoads(t, s))
}

func testDeleteExecutors(t *testing.T, b Backend) {
	ctx := t.Context()
	s := b.NewStore(t)
	require.NoError(t, s.RecordHeartbeat(ctx, namespace, executorA, heartbeat(types.ExecutorStatusACTIVE, nil)))
	require.NoError(t, s.RecordHeartbeat(ctx, namespace, executorB, heartbeat(types.ExecutorStatusACTIVE, nil)))

	require.NoError(t, s.DeleteExecutors(ctx, namespace, nil, store.NopGuard()), "an empty delete is a no-op")
	require.NoError(t, s.DeleteExecutors(ctx, namespace, []string{executorA, "missing-executor"}, store.NopGuard()))

	_, err := s.GetExecutorState(ctx, namespace, executorA)
	require.ErrorIs(t, err, store.ErrExecutorNotFound)
	_, err = s.GetExecutorState(ctx, namespace, executorB)
	require.NoError(t, err)
}

func testDeleteShardStats(t *testing.T, b Backend) {
	ctx := t.Context()
	s := b.NewStore(t)
	require.NoError(t, s.RecordHeartbeat(ctx, namespace, executorA, heartbeat(types.ExecutorStatusACTIVE, nil)))
	require.NoError(t, s.RecordShardStatisticsBatch(ctx, namespace, []store.ExecutorShardStatistics{
		{ExecutorID: executorA, Statistics: map[string]store.ShardStatistics{"shard-1": {SmoothedLoad: 1}, "shard-2": {SmoothedLoad: 2}}},
		{ExecutorID: executorB, Statistics: map[string]store.ShardStatistics{"shard-3": {SmoothedLoad: 3}}},
	}))

	require.NoError(t, s.DeleteShardStats(ctx, namespace, []string{"shard-1", "shard-3"}, store.NopGuard()))

	assert.Equal(t, map[string]float64{"shard-2": 2}, smoothedLoads(t, s))
	// Executor B had nothing but statistics, so it is gone with them.
	_, err := s.GetExecutorState(ctx, namespace, executorB)
	require.ErrorIs(t, err, store.ErrExecutorNotFound)
	_, err = s.GetExecutorState(ctx, namespace, executorA)
	require.NoError(t, err)
}

func testGuardedWrites(t *testing.T, b Backend) {
	ctx := t.Context()
	s := b.NewStore(t)
	failing := requireFailingGuard(t, b)
	require.NoError(t, s.RecordHeartbeat(ctx, namespace, executorA, heartbeat(types.ExecutorStatusACTIVE, nil)))
	require.NoError(t, s.RecordShardStatisticsBatch(ctx, namespace, []store.ExecutorShardStatistics{
		{ExecutorID: executorA, Statistics: map[string]store.ShardStatistics{"shard-1": {SmoothedLoad: 1}}},
	}))

	require.Error(t, s.DeleteExecutors(ctx, namespace, []string{executorA}, failing))
	require.Error(t, s.DeleteShardStats(ctx, namespace, []string{"shard-1"}, failing))

	_, err := s.GetExecutorState(ctx, namespace, executorA)
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{"shard-1": 1}, smoothedLoads(t, s))
}

func testDrainShards(t *testing.T, b Backend) {
	ctx := t.Context()
	s := b.NewStore(t)

	require.Error(t, s.DrainShards(ctx, namespace, []string{"bad/shard"}))
	require.NoError(t, s.DrainShards(ctx, namespace, []string{"shard-2", "shard-1"}))

	drained, err := s.GetDrainedShards(ctx, namespace)
	require.NoError(t, err)
	assert.Equal(t, []string{"shard-1", "shard-2"}, drained)

	removed, err := s.UndrainShards(ctx, namespace, []string{"shard-1", "shard-3"})
	require.NoError(t, err)
	assert.Equal(t, []string{"shard-1"}, removed)

	state, err := s.GetAssignmentState(ctx, namespace)
	require.NoError(t, err)
	assert.Equal(t, map[string]struct{}{"shard-2": {}}, state.DrainedShards)
}

func testDrainHosts(t *testing.T, b Backend) {
	ctx := t.Context()
	s := b.NewStore(t)
	drainedAt := time.Unix(1000, 0).UTC()

	require.Error(t, s.DrainHosts(ctx, namespace, []store.DrainedHost{{Hostname: ""}}))
	require.NoError(t, s.DrainHosts(ctx, namespace, []store.DrainedHost{
		{Hostname: "host-b", DrainedAt: drainedAt, Reason: "first"},
		{Hostname: "host-a"},
	}))
	// Draining again keeps the original record.
	require.NoError(t, s.DrainHosts(ctx, namespace, []store.DrainedHost{{Hostname: "host-b", Reason: "second"}}))

	hosts, err := s.GetDrainedHosts(ctx, namespace)
	require.NoError(t, err)
	require.Len(t, hosts, 2)
	assert.Equal(t, "host-a", hosts[0].Hostname)
	assert.False(t, hosts[0].DrainedAt.IsZero(), "a missing drain time is filled in")
	assert.Equal(t, store.DrainedHost{Hostname: "host-b", DrainedAt: drainedAt, Reason: "first"}, hosts[1])

	removed, err := s.UndrainHosts(ctx, namespace, []string{"host-a", "host-c"})
	require.NoError(t, err)
	assert.Equal(t, []string{"host-a"}, removed)
}

func testResetNamespace(t *testing.T, b Backend) {
	ctx := t.Context()
	s := b.NewStore(t)
	require.NoError(t, s.RecordHeartbeat(ctx, namespace, executorA, heartbeat(types.ExecutorStatusACTIVE, map[string]string{"a": "1"})))
	require.NoError(t, s.AssignShards(ctx, namespace, assignRequest(map[string]store.AssignedState{executorA: assignment(0, "shard-1")}), store.NopGuard()))
	require.NoError(t, s.DrainShards(ctx, namespace, []string{"shard-1"}))
	require.NoError(t, s.DrainHosts(ctx, namespace, []store.DrainedHost{{Hostname: "host-1"}}))

	removed, err := s.ResetNamespace(ctx, namespace)
	require.NoError(t, err)
	// Heartbeat time, status, reported shards, host metadata, one metadata key, the
	// assignment, the drained shard and the drained host.
	assert.Equal(t, int64(8), removed)

	state, err := s.GetState(ctx, namespace)
	require.NoError(t, err)
	assert.Empty(t, state.Executors)
	assert.Empty(t, state.ShardAssignments)
	assert.Empty(t, state.DrainedShards)
	assert.Empty(t, state.DrainedHosts)

	removed, err = s.ResetNamespace(ctx, namespace)
	require.NoError(t, err)
	assert.Zero(t, removed)
}

func testRanges(t *testing.T, b Backend) {
	ctx := t.Context()
	s := b.NewStore(t)

	ranges, err := s.ListRanges(ctx, namespace)
	require.NoError(t, err)
	assert.Empty(t, ranges)
	require.ErrorIs(t, s.AssignRanges(ctx, namespace, store.AssignRangesRequest{}, store.NopGuard()), store.ErrRangeNotImplemented)
}

func testSubscribeToNamespaceChanges(t *testing.T, b Backend) {
	ctx := t.Context()
	s := b.NewStore(t)

	changes, err := s.SubscribeToNamespaceChanges(namespace)
	require.NoError(t, err)
	// Subscribers are signalled up front so they read the state once before waiting.
	requireSignal(t, changes)

	require.NoError(t, s.RecordHeartbeat(ctx, namespace, executorA, heartbeat(types.ExecutorStatusACTIVE, nil)))
	requireNoSignal(t, changes, "a heartbeat without metadata changes")

	require.NoError(t, s.AssignShards(ctx, namespace, assignRequest(map[string]store.AssignedState{executorA: assignment(0, "shard-1")}), store.NopGuard()))
	requireSignal(t, changes)

	rewrite := assignment(assignedRevision(t, s, executorA), "shard-1")
	require.NoError(t, s.AssignShards(ctx, namespace, assignRequest(map[string]store.AssignedState{executorA: rewrite}), store.NopGuard()))
	requireNoSignal(t, changes, "rewriting an unchanged assignment")

	require.NoError(t, s.DrainShards(ctx, namespace, []string{"shard-1"}))
	requireSignal(t, changes)

	_, err = s.UndrainShards(ctx, namespace, []string{"shard-not-drained"})
	require.NoError(t, err)
	requireNoSignal(t, changes, "undraining a shard that is not drained")
}

func testSubscribeToExecutorStatusChanges(t *testing.T, b Backend) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s := b.NewStore(t)

	revisions, err := s.SubscribeToExecutorStatusChanges(ctx, namespace)
	require.NoError(t, err)

	// A watch-based subscription may start after the call returns, so status changes
	// are written until the first one is observed.
	statuses := []types.ExecutorStatus{types.ExecutorStatusACTIVE, types.ExecutorStatusDRAINING}
	var current types.ExecutorStatus
	deadline := time.Now().Add(signalTimeout)
	for observed := false; !observed; {
		require.True(t, time.Now().Before(deadline), "no status change was ever signalled")
		current = statuses[0]
		statuses[0], statuses[1] = statuses[1], statuses[0]
		require.NoError(t, s.RecordHeartbeat(ctx, namespace, executorA, heartbeat(current, nil)))
		select {
		case <-revisions:
			observed = true
		case <-time.After(quietPeriod / 4):
		}
	}
	// A slow delivery of an earlier write may still be in flight.
	drain(revisions, quietPeriod)

	require.NoError(t, s.RecordHeartbeat(ctx, namespace, executorA, heartbeat(current, nil)))
	requireNoSignal(t, revisions, "a heartbeat with an unchanged status")

	require.NoError(t, s.RecordHeartbeat(ctx, namespace, executorA, heartbeat(types.ExecutorStatusDRAINED, nil)))
	state, err := s.GetAssignmentState(ctx, namespace)
	require.NoError(t, err)
	select {
	case revision := <-revisions:
		assert.Positive(t, revision)
		assert.LessOrEqual(t, revision, state.Revision)
	case <-time.After(signalTimeout):
		require.Fail(t, "expected a status change signal")
	}

	require.NoError(t, s.DeleteExecutors(ctx, namespace, []string{executorA}, store.NopGuard()))
	requireSignal(t, revisions)

	cancel()
	require.Eventually(t, func() bool {
		select {
		case _, open := <-revisions:
			return !open
		default:
			return false
		}
	}, signalTimeout, time.Millisecond)
}

func heartbeat(status types.ExecutorStatus, metadata map[string]string) store.HeartbeatState {
	return store.HeartbeatState{
		LastHeartbeat: time.Unix(100, 0).UTC(),
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

func assignRequest(assignments map[string]store.AssignedState) store.AssignShardsRequest {
	return store.AssignShardsRequest{NewState: &store.NamespaceState{
		AssignmentState: store.AssignmentState{ShardAssignments: assignments},
	}}
}

func assignedRevision(t *testing.T, s store.Store, executorID string) int64 {
	t.Helper()
	state, err := s.GetAssignmentState(t.Context(), namespace)
	require.NoError(t, err)
	assigned, ok := state.ShardAssignments[executorID]
	require.True(t, ok, "executor %s has no assignment", executorID)
	return assigned.ModRevision
}

func smoothedLoads(t *testing.T, s store.Store) map[string]float64 {
	t.Helper()
	state, err := s.GetState(t.Context(), namespace)
	require.NoError(t, err)
	loads := make(map[string]float64, len(state.ShardStats))
	for shardID, stats := range state.ShardStats {
		loads[shardID] = stats.SmoothedLoad
	}
	return loads
}

func requireSignal[T any](t *testing.T, ch <-chan T) {
	t.Helper()
	select {
	case _, open := <-ch:
		require.True(t, open, "channel closed while waiting for a signal")
	case <-time.After(signalTimeout):
		require.Fail(t, "expected a signal")
	}
}

func requireNoSignal[T any](t *testing.T, ch <-chan T, after string) {
	t.Helper()
	select {
	case v := <-ch:
		require.Failf(t, "unexpected signal", "got %v after %s", v, after)
	case <-time.After(quietPeriod):
	}
}

// drain discards signals until none arrives for the given period.
func drain[T any](ch <-chan T, period time.Duration) {
	for {
		select {
		case <-ch:
		case <-time.After(period):
			return
		}
	}
}
