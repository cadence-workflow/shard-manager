package cache

import (
	"sync"

	"github.com/google/uuid"
)

type executorStateSubscriber struct {
	notifyCh chan struct{}
}

// executorStatePubSub manages subscriptions to executor state changes.
//
// Each subscriber has a buffered (size 1) channel. When a subscriber can't
// keep up, notifySubscribers drains the stale pending message and replaces it with
// the latest state, so the subscriber always catches up to the most recent
// state rather than being stuck on a stale intermediate state.
type executorStatePubSub struct {
	sync.RWMutex

	subscribers map[string]*executorStateSubscriber
}

func newExecutorStatePubSub() *executorStatePubSub {
	return &executorStatePubSub{
		subscribers: make(map[string]*executorStateSubscriber),
	}
}

// subscribe returns a channel that receives executor state updates.
func (p *executorStatePubSub) subscribe() (chan struct{}, func()) {
	uniqueID := uuid.New().String()
	subscriber := &executorStateSubscriber{
		notifyCh: make(chan struct{}, 1),
	}

	unSub := func() { p.unSubscribe(uniqueID) }

	p.Lock()
	defer p.Unlock()
	p.subscribers[uniqueID] = subscriber

	return subscriber.notifyCh, unSub
}

func (p *executorStatePubSub) unSubscribe(uniqueID string) {
	p.Lock()
	defer p.Unlock()
	delete(p.subscribers, uniqueID)
}

// notifySubscribers asynchronously sends a notification to all subscribers.
func (p *executorStatePubSub) notifySubscribers() {
	p.RLock()
	defer p.RUnlock()

	for _, sub := range p.subscribers {
		select {
		case sub.notifyCh <- struct{}{}:
		default:
		}
	}
}
