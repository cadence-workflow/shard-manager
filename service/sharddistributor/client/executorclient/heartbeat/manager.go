package heartbeat

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination manager_mock.go

import (
	"context"
	"fmt"
	"time"

	"github.com/uber-go/tally"
	"golang.org/x/sync/singleflight"

	"github.com/cadence-workflow/shard-manager/client/sharddistributorexecutor"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient/metricsconstants"
)

const drainingHeartbeatTimeout = 5 * time.Second

// minActiveHeartbeatTimeout floors the bound on a coalesced heartbeat RPC, so a
// very short configured interval cannot cut off heartbeats that would otherwise
// have succeeded.
const minActiveHeartbeatTimeout = 1 * time.Second

// StateProvider allows the heartbeat manager to pull dynamic state from the
// executor without knowing how the executor is implemented.
type StateProvider interface {
	GetShardStatusReports() map[string]*types.ShardStatusReport
	GetMetadata() map[string]string
}

// Manager owns the heartbeat RPC lifecycle: building requests, sending them
// through the shard-distributor client, coalescing concurrent callers via
// singleflight, and emitting the owned-shards gauge.
type Manager struct {
	client      sharddistributorexecutor.Client
	namespace   string
	executorID  string
	state       StateProvider
	hostMetrics tally.Scope
	sf          singleflight.Group
	// activeHeartbeatTimeout bounds a coalesced active heartbeat RPC. The RPC
	// is detached from the winning caller's context, so it needs its own
	// deadline.
	activeHeartbeatTimeout time.Duration
}

func NewManager(
	client sharddistributorexecutor.Client,
	namespace string,
	executorID string,
	state StateProvider,
	hostMetrics tally.Scope,
	heartbeatInterval time.Duration,
) *Manager {
	return &Manager{
		client:                 client,
		namespace:              namespace,
		executorID:             executorID,
		state:                  state,
		hostMetrics:            hostMetrics,
		activeHeartbeatTimeout: max(heartbeatInterval, minActiveHeartbeatTimeout),
	}
}

// Heartbeat sends a coalesced active heartbeat. Concurrent callers share a
// single in-flight RPC via singleflight.
//
// When shardID is non-empty, an optimistic check is performed: if the
// piggybacked response already contains the shard, it is returned
// immediately without a second RPC.
func (m *Manager) Heartbeat(ctx context.Context, shardID string) (map[string]*types.ShardAssignment, error) {
	assignments, shared, err := m.doHeartbeat(ctx)
	if err != nil {
		return nil, err
	}

	if !shared {
		return assignments, nil
	}

	if shardID != "" && assignments[shardID] != nil {
		return assignments, nil
	}

	// Since the heartbeat was shared, and we didn't find the shard in the first heartbeat,
	// the heartbeat might have started _before_ the shard was assigned, so we do another.
	// We know this was initiated _after_ we got the request, so the shard should be assigned.
	assignments, _, err = m.doHeartbeat(ctx)
	return assignments, err
}

// DrainingHeartbeat sends a one-off draining signal with a built-in timeout.
func (m *Manager) DrainingHeartbeat() error {
	ctx, cancel := context.WithTimeout(context.Background(), drainingHeartbeatTimeout)
	defer cancel()

	_, err := m.sendRPC(ctx, types.ExecutorStatusDRAINING)
	return err
}

func (m *Manager) doHeartbeat(ctx context.Context) (map[string]*types.ShardAssignment, bool, error) {
	result, err, shared := m.sf.Do("heartbeat", func() (any, error) {
		// Detach from the winning caller's context so one caller's
		// cancellation cannot abort the shared RPC for all piggybacking
		// callers, but give it a deadline of its own so a hung server cannot
		// wedge the singleflight indefinitely.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), m.activeHeartbeatTimeout)
		defer cancel()

		return m.sendRPC(ctx, types.ExecutorStatusACTIVE)
	})
	if err != nil {
		return nil, shared, err
	}
	return result.(map[string]*types.ShardAssignment), shared, nil
}

func (m *Manager) sendRPC(ctx context.Context, status types.ExecutorStatus) (map[string]*types.ShardAssignment, error) {
	reports := m.state.GetShardStatusReports()
	m.hostMetrics.Gauge(metricsconstants.ShardDistributorExecutorOwnedShards).Update(float64(len(reports)))

	request := &types.ExecutorHeartbeatRequest{
		Namespace:          m.namespace,
		ExecutorID:         m.executorID,
		Status:             status,
		ShardStatusReports: reports,
		Metadata:           m.state.GetMetadata(),
	}

	response, err := m.client.Heartbeat(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("send heartbeat: %w", err)
	}

	return response.ShardAssignments, nil
}
