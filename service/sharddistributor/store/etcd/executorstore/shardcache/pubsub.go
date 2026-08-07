package shardcache

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// executorStatePubSub manages subscriptions to assignment snapshots, which carry
// both executor state and the drained set.
//
// Each subscriber has a buffered (size 1) channel. When a subscriber can't
// keep up, publish drains the stale pending message and replaces it with
// the latest state, so the subscriber always catches up to the most recent
// state rather than being stuck on a stale intermediate state.
type executorStatePubSub struct {
	mu          sync.Mutex
	subscribers map[string]chan store.AssignmentSnapshot
	logger      log.Logger
	namespace   string
}

func newExecutorStatePubSub(logger log.Logger, namespace string) *executorStatePubSub {
	return &executorStatePubSub{
		subscribers: make(map[string]chan store.AssignmentSnapshot),
		logger:      logger,
		namespace:   namespace,
	}
}

// Subscribe returns a channel that receives executor state updates.
func (p *executorStatePubSub) subscribe(ctx context.Context) (chan store.AssignmentSnapshot, func()) {
	ch := make(chan store.AssignmentSnapshot, 1)
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
func (p *executorStatePubSub) publish(state store.AssignmentSnapshot) {
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
