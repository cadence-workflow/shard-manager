package shardcache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

func TestExecutorStatePubSub_SubscribeUnsubscribe(t *testing.T) {
	defer goleak.VerifyNone(t)
	pubsub := newExecutorStatePubSub(testlogger.New(t), "test-ns")

	ch, unsub := pubsub.subscribe(context.Background())
	assert.NotNil(t, ch)
	assert.Len(t, pubsub.subscribers, 1)

	unsub()
	assert.Len(t, pubsub.subscribers, 0)

	// Unsubscribe is idempotent
	unsub()
	assert.Len(t, pubsub.subscribers, 0)
}

func TestExecutorStatePubSub_Publish(t *testing.T) {
	defer goleak.VerifyNone(t)

	t.Run("no subscribers doesn't panic", func(t *testing.T) {
		pubsub := newExecutorStatePubSub(testlogger.New(t), "test-ns")
		require.NotPanics(t, func() {
			pubsub.publish(map[*store.ShardOwner][]string{})
		})
	})

	t.Run("multiple subscribers receive updates", func(t *testing.T) {
		pubsub := newExecutorStatePubSub(testlogger.New(t), "test-ns")
		ch1, unsub1 := pubsub.subscribe(context.Background())
		ch2, unsub2 := pubsub.subscribe(context.Background())
		defer unsub1()
		defer unsub2()

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

		pubsub.publish(testState)

		wg.Wait()
	})

	t.Run("slow consumer receives latest state", func(t *testing.T) {
		pubsub := newExecutorStatePubSub(testlogger.New(t), "test-ns")

		// Create slow subscriber
		ch, unsub := pubsub.subscribe(context.Background())
		defer unsub()

		// Four states will be published
		for i := range 4 {
			state := map[*store.ShardOwner][]string{
				{ExecutorID: fmt.Sprintf("exec-%d", i), Metadata: map[string]string{}}: {"shard-1"},
			}
			pubsub.publish(state)
		}
		// Last state should be the latest
		lastState := map[*store.ShardOwner][]string{
			{ExecutorID: "LAST_STATE_EXECUTOR", Metadata: map[string]string{}}: {"LAST_STATE_SHARD"},
		}
		pubsub.publish(lastState)

		// The subscriber receives the latest state
		got := <-ch
		assert.Equal(t, lastState, got)
	})
}
