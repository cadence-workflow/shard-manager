package hedged

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBudget(t *testing.T) {
	b := newBudget(func() float64 { return 0.5 })
	b.recordCall()
	assert.Equal(t, int(maxTokens), drain(b), "starts full and never exceeds the cap")

	b.recordCall()
	b.recordCall()
	assert.Equal(t, 1, drain(b), "two calls at a 0.5 share earn one hedge")

	off := newBudget(func() float64 { return 0 })
	assert.False(t, off.allowHedge(), "a zero share turns hedging off even with saved tokens")
}

// drain spends the budget and returns how many hedges it allowed.
func drain(b *budget) int {
	n := 0
	for b.allowHedge() {
		n++
	}
	return n
}
