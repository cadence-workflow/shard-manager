package shardcache

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

var emptyState = map[*store.ShardOwner][]string{}

func snapshotOf(state map[*store.ShardOwner][]string) func() map[*store.ShardOwner][]string {
	return func() map[*store.ShardOwner][]string { return state }
}

func TestExecutorStatePubSub_SubscribeUnsubscribe(t *testing.T) {
	defer goleak.VerifyNone(t)
	pubsub := newExecutorStatePubSub(testlogger.New(t), "test-ns", clock.NewMockedTimeSource())

	ch, unsub := pubsub.subscribe(snapshotOf(emptyState))
	assert.NotNil(t, ch)
	assert.Len(t, pubsub.subscribers, 1)

	// Drain initial state
	<-ch

	unsub()
	assert.Len(t, pubsub.subscribers, 0)

	// Unsubscribe is idempotent
	unsub()
	assert.Len(t, pubsub.subscribers, 0)
}

func TestExecutorStatePubSub_SubscribeDeliversInitialState(t *testing.T) {
	defer goleak.VerifyNone(t)

	pubsub := newExecutorStatePubSub(testlogger.New(t), "test-ns", clock.NewMockedTimeSource())
	initialState := map[*store.ShardOwner][]string{
		{ExecutorID: "exec-1", Metadata: map[string]string{}}: {"shard-1"},
	}

	ch, unsub := pubsub.subscribe(snapshotOf(initialState))
	defer unsub()

	got := <-ch
	assert.Equal(t, initialState, got)
}

func TestExecutorStatePubSub_PublishDoesNotDeadlock(t *testing.T) {
	defer goleak.VerifyNone(t)

	pubsub := newExecutorStatePubSub(testlogger.New(t), "test-ns", clock.NewMockedTimeSource())

	// subscribe seeds the channel with the initial state, filling the
	// single buffer slot. A subsequent publish must complete without
	// blocking — it drains the stale value and replaces it.
	ch, unsub := pubsub.subscribe(snapshotOf(emptyState))

	updateState := map[*store.ShardOwner][]string{
		{ExecutorID: "update", Metadata: map[string]string{}}: {"s2"},
	}

	done := make(chan struct{})
	go func() {
		pubsub.publish(func() map[*store.ShardOwner][]string { return updateState })
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publish blocked — deadlock detected")
	}

	// unSubscribe must also not hang (it needs p.mu).
	unsubDone := make(chan struct{})
	go func() {
		unsub()
		close(unsubDone)
	}()

	select {
	case <-unsubDone:
	case <-time.After(2 * time.Second):
		t.Fatal("unSubscribe blocked — deadlock detected")
	}

	// Drain whatever is left so goleak doesn't flag it.
	select {
	case <-ch:
	default:
	}
}

func TestExecutorStatePubSub_Publish(t *testing.T) {
	defer goleak.VerifyNone(t)

	t.Run("no subscribers doesn't panic", func(t *testing.T) {
		pubsub := newExecutorStatePubSub(testlogger.New(t), "test-ns", clock.NewMockedTimeSource())
		require.NotPanics(t, func() {
			pubsub.publish(func() map[*store.ShardOwner][]string { return map[*store.ShardOwner][]string{} })
		})
	})

	t.Run("multiple subscribers receive updates", func(t *testing.T) {
		pubsub := newExecutorStatePubSub(testlogger.New(t), "test-ns", clock.NewMockedTimeSource())
		ch1, unsub1 := pubsub.subscribe(snapshotOf(emptyState))
		ch2, unsub2 := pubsub.subscribe(snapshotOf(emptyState))
		defer unsub1()
		defer unsub2()

		// Drain initial states
		<-ch1
		<-ch2

		testState := map[*store.ShardOwner][]string{
			{ExecutorID: "exec-1", Metadata: map[string]string{}}: {"shard-1"},
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			state := <-ch1
			assert.Equal(t, testState, state)
			wg.Done()
		}()
		go func() {
			state := <-ch2
			assert.Equal(t, testState, state)
			wg.Done()
		}()
		time.Sleep(10 * time.Millisecond)

		pubsub.publish(func() map[*store.ShardOwner][]string { return testState })

		wg.Wait()
	})

	t.Run("slow consumer receives latest state", func(t *testing.T) {
		pubsub := newExecutorStatePubSub(testlogger.New(t), "test-ns", clock.NewMockedTimeSource())

		ch, unsub := pubsub.subscribe(snapshotOf(emptyState))
		defer unsub()

		// Drain initial state
		<-ch

		// Four states will be published
		for i := range 4 {
			state := map[*store.ShardOwner][]string{
				{ExecutorID: fmt.Sprintf("exec-%d", i), Metadata: map[string]string{}}: {"shard-1"},
			}
			pubsub.publish(func() map[*store.ShardOwner][]string { return state })
		}
		// Last state should be the latest
		lastState := map[*store.ShardOwner][]string{
			{ExecutorID: "LAST_STATE_EXECUTOR", Metadata: map[string]string{}}: {"LAST_STATE_SHARD"},
		}
		pubsub.publish(func() map[*store.ShardOwner][]string { return lastState })

		// The subscriber receives the latest state
		got := <-ch
		assert.Equal(t, lastState, got)
	})
}

func TestExecutorStatePubSub_DroppedUpdateLog(t *testing.T) {
	defer goleak.VerifyNone(t)

	timeSource := clock.NewMockedTimeSource()
	logger, logs := testlogger.NewObserved(t)
	pubsub := newExecutorStatePubSub(logger, "test-ns", timeSource)

	initialState := map[*store.ShardOwner][]string{
		{ExecutorID: "initial-executor", Metadata: map[string]string{}}: {"initial-shard"},
	}
	ch, unsub := pubsub.subscribe(snapshotOf(initialState))
	defer unsub()

	// The first publish replaces the initial assignment view and starts tracking update times.
	timeSource.Advance(time.Second)
	pubsub.publish(snapshotOf(map[*store.ShardOwner][]string{
		{ExecutorID: "executor-1", Metadata: map[string]string{}}: {"shard-1"},
	}))

	// Leave the replacement pending until the next publish.
	expectedInterval := 100 * time.Millisecond
	timeSource.Advance(expectedInterval)
	latestState := map[*store.ShardOwner][]string{
		{ExecutorID: "executor-2", Metadata: map[string]string{}}: {"shard-2"},
	}
	pubsub.publish(snapshotOf(latestState))

	dropLogs := logs.FilterMessage("subscriber not keeping up, dropping intermediate state update and replacing with latest").All()
	require.Len(t, dropLogs, 2)

	// The first drop has no previously tracked publish or pending update.
	firstDrop := dropLogs[0].ContextMap()
	assert.Equal(t, "test-ns", firstDrop["shard-namespace"])
	assert.Equal(t, time.Duration(0), firstDrop["state-update-publish-interval"])
	assert.Equal(t, time.Duration(0), firstDrop["subscriber-pending-update-duration"])

	secondDrop := dropLogs[1].ContextMap()
	assert.Equal(t, expectedInterval, secondDrop["state-update-publish-interval"])
	assert.Equal(t, expectedInterval, secondDrop["subscriber-pending-update-duration"])

	assert.Equal(t, latestState, <-ch)
}

func TestExecutorStatePubSub_ConsumerProgressResetsPendingUpdateDuration(t *testing.T) {
	defer goleak.VerifyNone(t)

	timeSource := clock.NewMockedTimeSource()
	logger, logs := testlogger.NewObserved(t)
	pubsub := newExecutorStatePubSub(logger, "test-ns", timeSource)

	ch, unsub := pubsub.subscribe(snapshotOf(emptyState))
	defer unsub()

	// Replace the initial assignment view, then consume the replacement to simulate progress.
	timeSource.Advance(time.Second)
	pubsub.publish(snapshotOf(map[*store.ShardOwner][]string{}))
	<-ch

	// Enqueue a new update, then publish twice without consuming it.
	timeSource.Advance(time.Second)
	pubsub.publish(snapshotOf(map[*store.ShardOwner][]string{}))

	publishInterval := time.Second
	timeSource.Advance(publishInterval)
	pubsub.publish(snapshotOf(map[*store.ShardOwner][]string{}))
	timeSource.Advance(publishInterval)
	pubsub.publish(snapshotOf(map[*store.ShardOwner][]string{}))

	dropLogs := logs.FilterMessage("subscriber not keeping up, dropping intermediate state update and replacing with latest").All()
	require.Len(t, dropLogs, 3)
	expectedPendingDuration := 2 * publishInterval
	assert.Equal(t, expectedPendingDuration, dropLogs[2].ContextMap()["subscriber-pending-update-duration"])
}
