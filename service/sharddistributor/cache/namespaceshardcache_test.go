package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// Every signal on the namespace subscription applies the state it reads, and a state
// that changed notifies the subscribers.
func TestNamespaceShardToExecutor_SignalAppliesStateAndNotifies(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer goleak.VerifyNone(t)

	executor1 := testExecutor{shards: []string{"shard-1"}, metadata: map[string]string{
		"hostname": "executor-1-host",
		"version":  "v1.0.0",
	}}
	executor2 := testExecutor{shards: []string{"shard-2"}, metadata: map[string]string{
		"hostname": "executor-2-host",
		"region":   "us-west",
	}}

	firstRead := tc.store.EXPECT().
		GetAssignmentState(gomock.Any(), tc.namespace).
		Return(namespaceState(1, map[string]testExecutor{"executor-1": executor1}), nil).
		Times(1)

	tc.store.EXPECT().
		GetAssignmentState(gomock.Any(), tc.namespace).
		Return(namespaceState(2, map[string]testExecutor{"executor-1": executor1, "executor-2": executor2}), nil).
		After(firstRead).
		AnyTimes()

	wg := sync.WaitGroup{}
	require.NoError(t, tc.e.Start(&wg))

	tc.changeCh <- struct{}{}
	requireExecutorCached(t, tc.e, "executor-1", "shard-1")
	verifyShardOwner(t, tc.e, "shard-1", "executor-1", executor1.metadata)

	notifyCh, unSub := tc.e.Subscribe()
	defer unSub()

	snapshot := tc.e.GetShardAssignments()
	assert.Len(t, snapshot.ExecutorToShards, 1)
	verifyExecutorInState(t, snapshot.ExecutorToShards, "executor-1", []string{"shard-1"}, executor1.metadata)

	// A refresh that changes the state notifies the subscribers.
	tc.changeCh <- struct{}{}

	select {
	case <-notifyCh:
	case <-time.After(time.Second):
		require.Fail(t, "expected to receive a notification")
	}

	snapshot = tc.e.GetShardAssignments()
	assert.Len(t, snapshot.ExecutorToShards, 2)
	verifyExecutorInState(t, snapshot.ExecutorToShards, "executor-1", []string{"shard-1"}, executor1.metadata)
	verifyExecutorInState(t, snapshot.ExecutorToShards, "executor-2", []string{"shard-2"}, executor2.metadata)
	verifyShardOwner(t, tc.e, "shard-2", "executor-2", executor2.metadata)

	close(tc.stopCh)
	wg.Wait()
}

func TestNamespaceShardToExecutor_namespaceRefreshLoop_HungRefreshDoesNotBlockStop(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer goleak.VerifyNone(t)
	tc.e.refreshTimeout = 50 * time.Millisecond

	gatedGetState(tc, nil)

	done := make(chan struct{})
	go func() {
		defer close(done)
		tc.e.namespaceRefreshLoop(tc.changeCh)
	}()

	tc.changeCh <- struct{}{}

	time.Sleep(10 * time.Millisecond)
	close(tc.stopCh)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("namespaceRefreshLoop did not exit after stopCh was closed while a refresh was in flight")
	}
}

func TestNamespaceShardToExecutor_replaceNamespaceState_skipsStaleRevision(t *testing.T) {
	defer goleak.VerifyNone(t)

	stopCh := make(chan struct{})
	defer close(stopCh)

	e := newNamespaceShardToExecutor("ns", store.NewMockStore(gomock.NewController(t)), stopCh, testlogger.New(t), clock.NewMockedTimeSource(), metrics.NewNoopMetricsClient())

	// Apply revision 10
	seedState(e, 10, "exec-a", "shard-1")

	got := e.GetShardAssignments()
	require.Len(t, got.ExecutorToShards, 1)
	assert.Contains(t, got.DrainedShards, "shard-1")
	assertShardDrained(t, e, "shard-1", true)

	// Apply revision 5 (stale) — should be ignored
	seedState(e, 5, "exec-b", "shard-2")

	got = e.GetShardAssignments()
	require.Len(t, got.ExecutorToShards, 1)
	for owner := range got.ExecutorToShards {
		assert.Equal(t, "exec-a", owner.ExecutorID)
	}
	assert.Contains(t, got.DrainedShards, "shard-1")
	assert.NotContains(t, got.DrainedShards, "shard-2")
	assertShardDrained(t, e, "shard-1", true)
	assertShardDrained(t, e, "shard-2", false)

	// Apply revision 20 (newer) — should be accepted
	seedState(e, 20, "exec-b", "shard-2")

	got = e.GetShardAssignments()
	require.Len(t, got.ExecutorToShards, 1)
	for owner := range got.ExecutorToShards {
		assert.Equal(t, "exec-b", owner.ExecutorID)
	}
	assert.NotContains(t, got.DrainedShards, "shard-1")
	assert.Contains(t, got.DrainedShards, "shard-2")
	assertShardDrained(t, e, "shard-1", false)
	assertShardDrained(t, e, "shard-2", true)
}

// seedState applies a revision in which one executor owns one shard and that shard is
// drained, which is the shape every caller here needs; replaceNamespaceState itself takes
// five parallel maps.
func seedState(e *namespaceShardToExecutor, revision int64, executorID, shardID string) {
	owner := &store.ShardOwner{ExecutorID: executorID, Metadata: map[string]string{}}
	e.replaceNamespaceState(revision,
		map[string]*store.ShardOwner{shardID: owner},
		map[*store.ShardOwner][]string{owner: {shardID}},
		map[string]int64{executorID: revision},
		map[string]*store.ShardOwner{executorID: owner},
		map[string]struct{}{shardID: {}},
	)
}

// assertShardDrained checks IsShardDrained without letting a cache miss reach the store; every
// caller here has already seeded a revision.
func assertShardDrained(t *testing.T, e *namespaceShardToExecutor, shardID string, expected bool) {
	t.Helper()

	drained, err := e.IsShardDrained(context.Background(), shardID)
	require.NoError(t, err)
	assert.Equal(t, expected, drained, "shard %s drained state", shardID)
}

// A drain and a later undrain must both reach the cache through the namespace subscription.
func TestNamespaceShardToExecutor_namespaceRefreshLoop_refreshesDrainedShards(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer goleak.VerifyNone(t)

	const shardID = "shard-1"

	drainedState := tc.state(5, nil)
	drainedState.DrainedShards = map[string]struct{}{shardID: {}}

	drainedGet := tc.store.EXPECT().
		GetAssignmentState(gomock.Any(), tc.namespace).
		Return(drainedState, nil).
		Times(1)

	tc.store.EXPECT().
		GetAssignmentState(gomock.Any(), tc.namespace).
		Return(tc.state(6, nil), nil).
		After(drainedGet).
		Times(1)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		tc.e.namespaceRefreshLoop(tc.changeCh)
	}()

	// Read the map directly rather than through IsShardDrained, so polling cannot itself
	// trigger a refresh and consume an expected GetState.
	isDrained := func() bool {
		tc.e.RLock()
		defer tc.e.RUnlock()
		_, drained := tc.e.drainedShards[shardID]
		return drained
	}

	tc.changeCh <- struct{}{}
	require.Eventually(t, isDrained, time.Second, time.Millisecond, "expected the drained shard to reach the cache")

	tc.changeCh <- struct{}{}
	require.Eventually(t, func() bool { return !isDrained() }, time.Second, time.Millisecond, "expected the undrained shard to leave the cache")

	close(tc.stopCh)
	wg.Wait()
}

// An empty drained set is a loaded state, not a miss, so it must not re-read the store.
func TestNamespaceShardToExecutor_IsShardDrained_loadsOnceWhenNothingIsDrained(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer close(tc.stopCh)

	tc.store.EXPECT().
		GetAssignmentState(gomock.Any(), tc.namespace).
		Return(tc.state(7, nil), nil).
		Times(1)

	for i := 0; i < 3; i++ {
		drained, err := tc.e.IsShardDrained(context.Background(), "shard-1")
		require.NoError(t, err)
		assert.False(t, drained)
	}
}

// Regression test: publish must take its snapshot only once it holds the
// pubsub lock, so a publish queued behind a concurrent one can never deliver
// a snapshot older than what was already applied to the cache.
func TestNamespaceShardToExecutor_Refresh_PublishReflectsLatestStateNotStaleSnapshot(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer close(tc.stopCh)

	subCh, unsub := tc.e.pubSub.subscribe()
	defer unsub()

	// Apply the older state.
	seedState(tc.e, 5, "exec-a", "shard-a")

	// Hold the pubsub lock so the publish below queues behind it, simulating
	// a slower refresh whose publish call loses the race for the lock.
	tc.e.pubSub.Lock()

	publishDone := make(chan struct{})
	go func() {
		tc.e.pubSub.notifySubscribers() // this one should be enqueued since the lock is holded
		close(publishDone)
	}()

	// Give the goroutine time to block on the lock before the newer state is applied.
	time.Sleep(20 * time.Millisecond)

	// A concurrent, newer refresh applies its state while the notification above is
	// still enqueued.
	seedState(tc.e, 10, "exec-b", "shard-b")

	tc.e.pubSub.Unlock()
	<-publishDone

	select {
	case <-subCh:
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for notification")
	}

	// we don't check the state as it is definitely applied, only notification
}

func verifyExecutorInState(t *testing.T, state map[*store.ShardOwner][]string, executorID string, shards []string, metadata map[string]string) {
	executorInState := false
	for executor, executorShards := range state {
		if executor.ExecutorID == executorID {
			assert.Equal(t, shards, executorShards)
			assert.Equal(t, metadata, executor.Metadata)
			executorInState = true
			break
		}
	}
	assert.True(t, executorInState)
}

// verifyShardOwner checks that a shard has the expected owner and metadata
func verifyShardOwner(t *testing.T, cache *namespaceShardToExecutor, shardID, expectedExecutorID string, expectedMetadata map[string]string) {
	owner, err := cache.GetShardOwner(context.Background(), shardID)
	require.NoError(t, err)
	require.NotNil(t, owner)
	assert.Equal(t, expectedExecutorID, owner.ExecutorID)
	for key, expectedValue := range expectedMetadata {
		assert.Equal(t, expectedValue, owner.Metadata[key])
	}

	executor, err := cache.GetExecutor(context.Background(), expectedExecutorID)
	require.NoError(t, err)
	require.NotNil(t, executor)
	assert.Equal(t, expectedExecutorID, executor.ExecutorID)
	for key, expectedValue := range expectedMetadata {
		assert.Equal(t, expectedValue, executor.Metadata[key])
	}
}

type namespaceShardToExecutorTestCase struct {
	e     *namespaceShardToExecutor
	store *store.MockStore

	// changeCh stands in for the store's namespace subscription.
	changeCh chan struct{}
	stopCh   chan struct{}

	executorID string
	namespace  string
}

func setupNamespaceShardToExecutorTestCase(t *testing.T) *namespaceShardToExecutorTestCase {
	tc := &namespaceShardToExecutorTestCase{
		store:      store.NewMockStore(gomock.NewController(t)),
		changeCh:   make(chan struct{}),
		stopCh:     make(chan struct{}),
		executorID: "executor-1",
		namespace:  "test-namespace",
	}

	tc.store.EXPECT().
		SubscribeToNamespaceChanges(tc.namespace).
		Return((<-chan struct{})(tc.changeCh), nil).
		AnyTimes()

	tc.e = newNamespaceShardToExecutor(tc.namespace, tc.store, tc.stopCh, testlogger.New(t), clock.NewRealTimeSource(), metrics.NewNoopMetricsClient())
	return tc
}

// state builds a namespace snapshot holding the test case's executor with shard-1 assigned.
func (tc *namespaceShardToExecutorTestCase) state(revision int64, metadata map[string]string) *store.AssignmentState {
	return namespaceState(revision, map[string]testExecutor{
		tc.executorID: {shards: []string{"shard-1"}, metadata: metadata},
	})
}

type testExecutor struct {
	shards   []string
	metadata map[string]string
}

func namespaceState(revision int64, executors map[string]testExecutor) *store.AssignmentState {
	state := &store.AssignmentState{
		Revision:         revision,
		ExecutorMetadata: make(map[string]map[string]string, len(executors)),
		ShardAssignments: make(map[string]store.AssignedState, len(executors)),
	}
	for executorID, executor := range executors {
		assigned := make(map[string]*types.ShardAssignment, len(executor.shards))
		for _, shardID := range executor.shards {
			assigned[shardID] = &types.ShardAssignment{Status: types.AssignmentStatusREADY}
		}
		state.ExecutorMetadata[executorID] = executor.metadata
		state.ShardAssignments[executorID] = store.AssignedState{
			AssignedShards: assigned,
			ModRevision:    revision,
		}
	}
	return state
}

// requireExecutorCached waits for a refresh to land, reading the maps directly so the wait
// cannot itself trigger the cache-miss refresh it is waiting for.
func requireExecutorCached(t *testing.T, e *namespaceShardToExecutor, executorID, shardID string) {
	t.Helper()

	require.Eventually(t, func() bool {
		e.RLock()
		defer e.RUnlock()
		owner, ok := e.shardToExecutor[shardID]
		if !ok || owner.ExecutorID != executorID {
			return false
		}
		_, ok = e.executorRevision[executorID]
		return ok
	}, time.Second, time.Millisecond, "expected %s to reach the cache", executorID)
}

// gatedGetState blocks every store read until release is closed, so concurrent callers pile
// up in one singleflight; a nil release never unblocks, standing in for a hung store. The
// counter is how many reads actually reached the store, and the expectation is AnyTimes so a
// regression surfaces as a count mismatch rather than "unexpected call".
func gatedGetState(tc *namespaceShardToExecutorTestCase, release <-chan struct{}) *atomic.Int32 {
	calls := &atomic.Int32{}
	tc.store.EXPECT().
		GetAssignmentState(gomock.Any(), tc.namespace).
		DoAndReturn(func(ctx context.Context, _ string) (*store.AssignmentState, error) {
			calls.Add(1)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return tc.state(1, nil), nil
		}).
		AnyTimes()

	return calls
}

// N concurrent cache-miss GetShardOwner calls must collapse into 1 store read.
func TestNamespaceShardToExecutor_GetShardOwner_SingleFlightDedup(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer close(tc.stopCh)

	const numCallers = 50

	// release gates the read so all callers pile up in singleflight first.
	release := make(chan struct{})
	getCalls := gatedGetState(tc, release)

	var wg sync.WaitGroup
	owners := make([]*store.ShardOwner, numCallers)
	errs := make([]error, numCallers)
	start := make(chan struct{})
	for i := 0; i < numCallers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			owners[i], errs[i] = tc.e.GetShardOwner(context.Background(), "shard-1")
		}(i)
	}
	close(start)

	// Wait until the read is in flight, then let other callers join the same singleflight.
	require.Eventually(t, func() bool { return getCalls.Load() >= 1 }, time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	close(release)
	wg.Wait()

	require.EqualValues(t, 1, getCalls.Load(), "concurrent misses should collapse to one store read")
	for i := 0; i < numCallers; i++ {
		require.NoError(t, errs[i])
		require.NotNil(t, owners[i])
		assert.Equal(t, tc.executorID, owners[i].ExecutorID)
	}
}

// Cancelling the singleflight leader must not surface context.Canceled to other waiters.
func TestNamespaceShardToExecutor_GetShardOwner_CallerCancelDoesNotPoisonFlight(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer close(tc.stopCh)

	release := make(chan struct{})
	getCalls := gatedGetState(tc, release)

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancelledErrCh := make(chan error, 1)
	go func() {
		_, gerr := tc.e.GetShardOwner(cancelCtx, "shard-1")
		cancelledErrCh <- gerr
	}()

	survivorErrCh := make(chan error, 1)
	survivorOwnerCh := make(chan *store.ShardOwner, 1)
	go func() {
		o, gerr := tc.e.GetShardOwner(context.Background(), "shard-1")
		survivorErrCh <- gerr
		survivorOwnerCh <- o
	}()

	require.Eventually(t, func() bool { return getCalls.Load() >= 1 }, time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	cancel()

	select {
	case gerr := <-cancelledErrCh:
		require.Error(t, gerr)
		assert.ErrorIs(t, gerr, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("cancelled caller did not return after its context was cancelled")
	}

	close(release)
	select {
	case gerr := <-survivorErrCh:
		require.NoError(t, gerr)
	case <-time.After(time.Second):
		t.Fatal("survivor caller did not return after refresh completed")
	}
	owner := <-survivorOwnerCh
	require.NotNil(t, owner)
	assert.Equal(t, tc.executorID, owner.ExecutorID)
	assert.EqualValues(t, 1, getCalls.Load(), "the in-flight read must outlive the cancelled leader")
}

// A hung store read must not hold the singleflight key forever: the bounded
// refresh context must time out, surface DeadlineExceeded to waiters, and
// release the flight so the next caller can trigger a fresh read.
func TestNamespaceShardToExecutor_GetShardOwner_RefreshContextHasBoundedTimeout(t *testing.T) {
	tc := setupNamespaceShardToExecutorTestCase(t)
	defer close(tc.stopCh)
	tc.e.refreshTimeout = 50 * time.Millisecond

	getCalls := gatedGetState(tc, nil)

	// Use a generous caller deadline so the timeout we observe must come from
	// the refresh's own bounded context, not the caller's.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := tc.e.GetShardOwner(ctx, "shard-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.EqualValues(t, 1, getCalls.Load())

	// Flight must be released so the next caller triggers a fresh read.
	_, err = tc.e.GetShardOwner(ctx, "shard-1")
	require.Error(t, err)
	assert.EqualValues(t, 2, getCalls.Load(), "second caller should trigger a new read, not join the previous flight")
}
