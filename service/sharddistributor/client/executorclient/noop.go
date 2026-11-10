package executorclient

import (
	"context"

	"github.com/cadence-workflow/shard-manager/common/types"
)

// noopExecutor is an Executor implementation used when no shard-distributor
// config is provided. It acts as if all task lists are locally assigned: it
// never contacts the shard distributor and never manages shard processors.
type noopExecutor[SP ShardProcessor] struct{}

// NewNoopExecutor returns an Executor that is a no-op. Use this when the
// shard-distributor config is absent so that the rest of the matching engine
// can operate using the local hash-ring exclusively.
func NewNoopExecutor[SP ShardProcessor]() Executor[SP] {
	return &noopExecutor[SP]{}
}

func (e *noopExecutor[SP]) Start(_ context.Context)         {}
func (e *noopExecutor[SP]) Stop()                           {}
func (e *noopExecutor[SP]) GetNamespace() string            { return "" }
func (e *noopExecutor[SP]) GetExecutorID() string           { return "" }
func (e *noopExecutor[SP]) SetMetadata(_ map[string]string) {}
func (e *noopExecutor[SP]) GetMetadata() map[string]string  { return nil }
func (e *noopExecutor[SP]) GetShardStatusReports() map[string]*types.ShardStatusReport {
	return map[string]*types.ShardStatusReport{}
}

// GetShardProcess always returns ErrShardProcessNotFound so that callers
// fall back to local hash-ring ownership checks.
func (e *noopExecutor[SP]) GetShardProcess(_ context.Context, _ string) (SP, error) {
	var zero SP
	return zero, ErrShardProcessNotFound
}
