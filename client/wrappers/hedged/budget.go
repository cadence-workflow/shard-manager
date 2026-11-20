package hedged

import "sync"

// maxTokens caps saved hedges, so a quiet period cannot fund a long burst of hedges.
const maxTokens = 10.0

// budget is a token bucket that every call refills by a share of a hedge, share can change dynamically at runtime.
// this is similar to x/time/rate, which refills over time rather than per call, so it is not used.
type budget struct {
	share func() float64
	// A mutex is cheap next to the network call it guards, and x/time/rate also locks one on every call.
	mu     sync.Mutex
	tokens float64
}

// newBudget starts full, so a new client can hedge before it has made enough calls to earn a hedge.
func newBudget(share func() float64) *budget {
	return &budget{share: share, tokens: maxTokens}
}

// recordCall adds one call's share of a hedge, up to maxTokens.
func (b *budget) recordCall() {
	b.mu.Lock()
	defer b.mu.Unlock()
	tokensToAdd := max(b.share(), 0)
	b.tokens = min(b.tokens+tokensToAdd, maxTokens)
}

// allowHedge spends one hedge and reports whether there was one to spend.
func (b *budget) allowHedge() bool {
	// A zero share turns hedging off (no bucket).
	if b.share() <= 0 {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
