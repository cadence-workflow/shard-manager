package shardcache

import (
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

type executorStateSubscriber struct {
	updates            chan store.AssignmentSnapshot
	pendingUpdateSince time.Time
}

// executorStatePubSub manages subscriptions to assignment snapshots, which carry
// both executor state and the drained set.
//
// Each subscriber has a buffered (size 1) channel. When a subscriber can't
// keep up, publish drains the stale pending message and replaces it with
// the latest state, so the subscriber always catches up to the most recent
// state rather than being stuck on a stale intermediate state.
type executorStatePubSub struct {
	mu              sync.Mutex
	subscribers     map[string]*executorStateSubscriber
	logger          log.Logger
	namespace       string
	timeSource      clock.TimeSource
	lastPublishedAt time.Time
}

func newExecutorStatePubSub(logger log.Logger, namespace string, timeSource clock.TimeSource) *executorStatePubSub {
	return &executorStatePubSub{
		subscribers: make(map[string]*executorStateSubscriber),
		logger:      logger,
		namespace:   namespace,
		timeSource:  timeSource,
	}
}

// subscribe returns a channel that receives assignment snapshot updates.
// snapshot is called under p.mu — it must not re-acquire p.mu.
func (p *executorStatePubSub) subscribe(snapshot func() store.AssignmentSnapshot) (chan store.AssignmentSnapshot, func()) {
	uniqueID := uuid.New().String()
	subscriber := &executorStateSubscriber{
		updates: make(chan store.AssignmentSnapshot, 1),
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.subscribers[uniqueID] = subscriber
	subscriber.updates <- snapshot()

	unSub := func() {
		p.unSubscribe(uniqueID)
	}

	return subscriber.updates, unSub
}

func (p *executorStatePubSub) unSubscribe(uniqueID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.subscribers, uniqueID)
}

// publish sends the latest state to all subscribers.
// snapshot is called under p.mu — it must not re-acquire p.mu.
func (p *executorStatePubSub) publish(snapshot func() store.AssignmentSnapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.timeSource.Now()

	state := snapshot()
	for _, sub := range p.subscribers {
		select {
		case sub.updates <- state:
			sub.pendingUpdateSince = now
		default:
			// Preserve pendingUpdateSince when we drain the pending update ourselves.
			// Reset it if the consumer drained the update concurrently.
			select {
			case <-sub.updates:
				if !sub.pendingUpdateSince.IsZero() {
					p.logDroppedUpdate(sub, now)
				}
			default:
				sub.pendingUpdateSince = now
			}
			sub.updates <- state
			if sub.pendingUpdateSince.IsZero() {
				sub.pendingUpdateSince = now
			}
		}
	}

	p.lastPublishedAt = now
}

func (p *executorStatePubSub) logDroppedUpdate(sub *executorStateSubscriber, now time.Time) {
	var publishInterval time.Duration
	if !p.lastPublishedAt.IsZero() {
		publishInterval = now.Sub(p.lastPublishedAt)
	}
	var pendingUpdateDuration time.Duration
	if !sub.pendingUpdateSince.IsZero() {
		pendingUpdateDuration = now.Sub(sub.pendingUpdateSince)
	}
	p.logger.Warn(
		"subscriber not keeping up, dropping intermediate state update and replacing with latest",
		tag.ShardNamespace(p.namespace),
		tag.StateUpdatePublishInterval(publishInterval),
		tag.SubscriberPendingUpdateDuration(pendingUpdateDuration),
	)
}
