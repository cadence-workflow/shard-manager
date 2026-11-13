package memory

import (
	"fmt"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// Txn is the transaction a store.GuardFunc receives from this store. A guard attaches
// preconditions with If, and the store checks them under its lock immediately before
// applying the write, so a guarded write lands only while every condition holds.
type Txn struct {
	conditions []func() bool
}

// If adds a precondition and returns the transaction for chaining.
func (t *Txn) If(condition func() bool) *Txn {
	t.conditions = append(t.conditions, condition)
	return t
}

// checkGuard reports whether every precondition the guard attaches holds. It must be
// called with the store lock held, so no other write can land between the check and
// the write it protects.
func checkGuard(guard store.GuardFunc) (bool, error) {
	guarded, err := guard(&Txn{})
	if err != nil {
		return false, fmt.Errorf("apply transaction guard: %w", err)
	}
	txn, ok := guarded.(*Txn)
	if !ok {
		return false, fmt.Errorf("guard function returned invalid transaction type %T", guarded)
	}
	for _, condition := range txn.conditions {
		if !condition() {
			return false, nil
		}
	}
	return true, nil
}

// requireGuard is checkGuard for writes that treat a failed precondition as an error.
func requireGuard(guard store.GuardFunc) error {
	held, err := checkGuard(guard)
	if err != nil {
		return err
	}
	if !held {
		return errGuardFailed
	}
	return nil
}
