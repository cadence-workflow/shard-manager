package spectatorclient

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/clientcommon/tag"
	csync "github.com/cadence-workflow/shard-manager/service/sharddistributor/client/spectatorclient/sync"
)

// ShardOwner contains information about the executor that owns a shard
type ShardOwner struct {
	ExecutorID string
	Metadata   map[string]string
}

// spectatorImpl answers shard owner lookups from the registry. The stream watcher keeps the registry
// current.
type spectatorImpl struct {
	namespace string
	client    sharddistributor.Client
	logger    *zap.Logger
	registry  *registry
	watcher   *streamWatcher

	// Signal to notify when first state is received
	firstStateSignal *csync.ResettableSignal
}

func (s *spectatorImpl) Start(ctx context.Context) error {
	s.watcher.Start(ctx)
	return nil
}

func (s *spectatorImpl) Stop() {
	s.watcher.Stop()
	// Close the firstStateSignal to unblock any goroutines waiting for first state
	s.firstStateSignal.Done()
}

// GetShardOwner returns the full owner information including metadata for a given shard.
// It first waits for the initial state to be received, then checks the cache.
// If not found in cache, it falls back to querying the shard distributor directly.
//
// A drained shard is refused from the local state without an RPC. Drain is checked
// before ownership so the answer matches the shard distributor, which also refuses a
// drained shard while it still has an owner.
func (s *spectatorImpl) GetShardOwner(ctx context.Context, shardKey string) (*ShardOwner, error) {
	// Wait for first state to be received to avoid flooding shard distributor on startup
	if err := s.firstStateSignal.Wait(ctx); err != nil {
		return nil, fmt.Errorf("wait for first state: %w", err)
	}

	// Check cache first
	owner, drained := s.registry.lookup(shardKey)

	if drained {
		return nil, &types.ShardDrainedError{
			Namespace: s.namespace,
			ShardKey:  shardKey,
		}
	}

	if owner != nil {
		return owner, nil
	}

	// Cache miss - fall back to RPC call
	s.logger.Debug("Shard not found in cache, querying shard distributor",
		zap.String(tag.ShardKey, shardKey),
		zap.String(tag.Namespace, s.namespace))

	response, err := s.client.GetShardOwner(ctx, &types.GetShardOwnerRequest{
		Namespace: s.namespace,
		ShardKey:  shardKey,
	})
	if err != nil {
		return nil, fmt.Errorf("get shard owner from shard distributor: %w", err)
	}

	return &ShardOwner{
		ExecutorID: response.Owner,
		Metadata:   response.Metadata,
	}, nil
}
