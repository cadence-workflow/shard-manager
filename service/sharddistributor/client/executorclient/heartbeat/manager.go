package heartbeat

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination manager_mock.go

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/uber-go/tally"
	"go.uber.org/yarpc/yarpcerrors"
	"golang.org/x/sync/singleflight"

	"github.com/cadence-workflow/shard-manager/client/sharddistributorexecutor"
	"github.com/cadence-workflow/shard-manager/client/wrappers/timeout"
	"github.com/cadence-workflow/shard-manager/common"
	"github.com/cadence-workflow/shard-manager/common/backoff"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient/metricsconstants"
)

const drainingHeartbeatTimeout = 5 * time.Second

const (
	// activeHeartbeatBudget bounds a whole flight, so blocked callers never wait a full client timeout.
	activeHeartbeatBudget = timeout.ShardDistributorExecutorDefaultTimeout / 2

	minActiveAttemptTimeout = 1 * time.Second
)

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
	client            sharddistributorexecutor.Client
	namespace         string
	executorID        string
	hostName          string
	state             StateProvider
	metrics           tally.Scope
	hostMetrics       tally.Scope
	sf                singleflight.Group
	retrier           *backoff.ThrottleRetry
	heartbeatInterval time.Duration
}

func NewManager(
	client sharddistributorexecutor.Client,
	namespace string,
	executorID string,
	hostName string,
	state StateProvider,
	metrics tally.Scope,
	hostMetrics tally.Scope,
	heartbeatInterval time.Duration,
) *Manager {
	return &Manager{
		client:            client,
		namespace:         namespace,
		executorID:        executorID,
		hostName:          hostName,
		state:             state,
		metrics:           metrics,
		hostMetrics:       hostMetrics,
		retrier:           newRetrier(),
		heartbeatInterval: heartbeatInterval,
	}
}

func newRetrier() *backoff.ThrottleRetry {
	return backoff.NewThrottleRetry(
		backoff.WithRetryPolicy(common.CreateShardDistributorServiceRetryPolicy()),
		backoff.WithRetryableError(func(err error) bool {
			return common.IsServiceTransientError(err) ||
				errors.Is(err, context.DeadlineExceeded) ||
				yarpcerrors.IsDeadlineExceeded(err)
		}),
	)
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
	led := false
	result, err, shared := m.sf.Do("heartbeat", func() (any, error) {
		led = true
		return m.activeFlight(ctx)
	})
	if !led {
		m.metrics.Counter(metricsconstants.ShardDistributorExecutorHeartbeatCallersJoined).Inc(1)
	}
	if err != nil {
		return nil, shared, err
	}
	return result.(map[string]*types.ShardAssignment), shared, nil
}

// activeFlight is detached from the winning caller's context so its cancellation
// cannot abort the RPC for every joined caller.
func (m *Manager) activeFlight(ctx context.Context) (map[string]*types.ShardAssignment, error) {
	budgetCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), activeHeartbeatBudget)
	defer cancel()

	// Metrics
	m.metrics.Counter(metricsconstants.ShardDistributorExecutorHeartbeatFlights).Inc(1)
	start := time.Now()
	defer func() {
		m.metrics.
			Histogram(metricsconstants.ShardDistributorExecutorHeartbeatFlightLatency, metricsconstants.ShardDistributorExecutorLatencyBuckets).
			RecordDuration(time.Since(start))
	}()

	// Do the call
	var assignments map[string]*types.ShardAssignment
	attempts := 0
	err := m.retrier.Do(budgetCtx, func(ctx context.Context) error {
		if attempts > 0 {
			m.metrics.Counter(metricsconstants.ShardDistributorExecutorHeartbeatRetries).Inc(1)
		}
		attempts++
		var err error
		assignments, err = m.activeAttempt(ctx)
		return err
	})
	if err == nil {
		return assignments, nil
	}

	// Error observability
	m.metrics.Counter(metricsconstants.ShardDistributorExecutorHeartbeatFlightFailures).Inc(1)
	if budgetCtx.Err() != nil {
		m.metrics.Counter(metricsconstants.ShardDistributorExecutorHeartbeatBudgetExhausted).Inc(1)
		return nil, fmt.Errorf("heartbeat budget %s exhausted after %d attempts: %w", activeHeartbeatBudget, attempts, err)
	}
	return nil, err
}

func (m *Manager) activeAttempt(ctx context.Context) (map[string]*types.ShardAssignment, error) {
	// Capped at half the budget so a hung attempt leaves room for another.
	attemptTimeout := min(max(m.heartbeatInterval, minActiveAttemptTimeout), activeHeartbeatBudget/2)
	ctx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()

	m.metrics.Counter(metricsconstants.ShardDistributorExecutorHeartbeatAttempts).Inc(1)
	return m.sendRPC(ctx, types.ExecutorStatusACTIVE)
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
	if m.hostName != "" {
		request.HostMetadata = &types.HostMetadata{HostName: m.hostName}
	}

	response, err := m.client.Heartbeat(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("send heartbeat: %w", err)
	}

	return response.ShardAssignments, nil
}
