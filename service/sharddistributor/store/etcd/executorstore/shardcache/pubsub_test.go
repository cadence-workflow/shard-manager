package shardcache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
)

func TestExecutorStatePubSub_SubscribeUnsubscribeWorks(t *testing.T) {
	defer goleak.VerifyNone(t)
	pubsub := newExecutorStatePubSub(testlogger.New(t), "test-ns")

	ch, unsub := pubsub.subscribe()
	assert.NotNil(t, ch)
	assert.Len(t, pubsub.subscribers, 1)

	unsub()
	assert.Empty(t, pubsub.subscribers)

	// Unsubscribe is idempotent
	unsub()
	assert.Empty(t, pubsub.subscribers)
}

func TestExecutorStatePubSub_NotificationsSent(t *testing.T) {
	defer goleak.VerifyNone(t)

	pubsub := newExecutorStatePubSub(testlogger.New(t), "test-ns")

	var receivedCh1, receivedCh2 bool
	ch1, unsub1 := pubsub.subscribe()
	defer unsub1()
	ch2, unsub2 := pubsub.subscribe()
	defer unsub2()

	timer := time.NewTimer(time.Second)
	defer timer.Stop()

	pubsub.notifySubscribers()

	for !(receivedCh1 && receivedCh2) {
		select {
		case <-ch1:
			receivedCh1 = true
		case <-ch2:
			receivedCh2 = true
		case <-timer.C:
			require.Fail(t, "timed out waiting for receiving")
		}
	}

	assert.True(t, receivedCh1)
	assert.True(t, receivedCh2)
}

func TestExecutorStatePubSub_NotificationsAreNotBlockedWhenOneReaderIsStuck(t *testing.T) {
	defer goleak.VerifyNone(t)

	pubsub := newExecutorStatePubSub(testlogger.New(t), "test-ns")

	var receivedCh2 int

	// we intentionally won't read ch1 to make sure ch2 is still receiving
	_, unsub1 := pubsub.subscribe()
	defer unsub1()

	ch2, unsub2 := pubsub.subscribe()
	defer unsub2()

	timer := time.NewTimer(time.Second)
	defer timer.Stop()

	pubsub.notifySubscribers()

	for receivedCh2 < 3 {
		select {
		case <-ch2:
			receivedCh2++
			pubsub.notifySubscribers()
		case <-timer.C:
			require.Fail(t, "timed out waiting for receiving")
		}
	}

	assert.Equal(t, 3, receivedCh2)
}
