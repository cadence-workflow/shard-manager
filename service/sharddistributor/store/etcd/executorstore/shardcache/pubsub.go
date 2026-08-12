package shardcache

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// executorStatePubSub manages subscriptions to executor state changes.
//
// Each subscriber has a buffered (size 1) channel. When a subscriber can't
// keep up, publish drains the stale pending message and replaces it with
// the latest state, so the subscriber always catches up to the most recent
// state rather than being stuck on a stale intermediate state.
type executorStatePubSub struct {
	mu          sync.Mutex
	subscribers map[string]chan map[*store.ShardOwner][]string
	logger      log.Logger
	namespace   string
}

func newExecutorStatePubSub(logger log.Logger, namespace string) *executorStatePubSub {
	return &executorStatePubSub{
		subscribers: make(map[string]chan map[*store.ShardOwner][]string),
		logger:      logger,
		namespace:   namespace,
	}
}

// Subscribe returns a channel that receives executor state updates.
func (p *executorStatePubSub) subscribe(ctx context.Context) (chan map[*store.ShardOwner][]string, func()) {
	ch := make(chan map[*store.ShardOwner][]string, 1)
	uniqueID := uuid.New().String()

	p.mu.Lock()
	defer p.mu.Unlock()
	p.subscribers[uniqueID] = ch

	unSub := func() {
		p.unSubscribe(uniqueID)
	}

	return ch, unSub
}

func (p *executorStatePubSub) unSubscribe(uniqueID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.subscribers, uniqueID)
}

// Publish sends the state to all subscribers (non-blocking).
// If a subscriber already has a pending message, it is drained and replaced
// with the new state so the subscriber always sees the latest.
func (p *executorStatePubSub) publish(state map[*store.ShardOwner][]string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, sub := range p.subscribers {
		select {
		case sub <- state:
		default:
			// Drain the stale pending message and replace with the latest.
			p.logger.Warn("subscriber not keeping up, dropping intermediate state update and replacing with latest", tag.ShardNamespace(p.namespace))
			select {
			case <-sub:
			default:
			}
			sub <- state
		}
	}
}
