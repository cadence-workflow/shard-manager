package load

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber-go/tally"
	"go.uber.org/mock/gomock"
	"go.uber.org/yarpc"
	"go.uber.org/zap/zaptest"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
	canarymetrics "github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/metrics"
)

var (
	convergedState = namespaceState(2, map[string]string{"1": "executor-a", "2": "executor-b"})
	staleState     = namespaceState(1, map[string]string{"1": "executor-old", "2": "executor-b"})
)

func TestConvergenceCheckerAwait(t *testing.T) {
	unchanged := &types.GetNamespaceStateIfNewerResponse{AssignmentRevision: 1}

	tests := []struct {
		name string
		// responses are returned to consecutive polls, the last one repeating.
		responses        []*types.GetNamespaceStateIfNewerResponse
		wantPolls        int
		wantConverged    bool
		wantTimeoutCount int64
	}{
		{
			name:          "cache already agrees",
			responses:     []*types.GetNamespaceStateIfNewerResponse{convergedState},
			wantPolls:     1,
			wantConverged: true,
		},
		{
			name:          "cache catches up",
			responses:     []*types.GetNamespaceStateIfNewerResponse{staleState, unchanged, convergedState},
			wantPolls:     3,
			wantConverged: true,
		},
		{
			name:             "stale cache times out",
			responses:        []*types.GetNamespaceStateIfNewerResponse{staleState, unchanged},
			wantTimeoutCount: 1,
		},
		{
			name:      "draining the switcher shard stops waiting",
			responses: []*types.GetNamespaceStateIfNewerResponse{staleState, drainedState()},
			wantPolls: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := sharddistributor.NewMockClient(ctrl)
			polls := 0
			var revision int64
			call := client.EXPECT().GetNamespaceStateIfNewer(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, req *types.GetNamespaceStateIfNewerRequest, _ ...yarpc.CallOption) (*types.GetNamespaceStateIfNewerResponse, error) {
					assert.Equal(t, testNamespace, req.Namespace)
					assert.Equal(t, revision, req.AssignmentRevision, "a poll must send the revision of the last state received")
					resp := tt.responses[min(polls, len(tt.responses)-1)]
					polls++
					if resp.State != nil {
						revision = resp.AssignmentRevision
					}
					return resp, nil
				},
			).MinTimes(1)
			if tt.wantPolls > 0 {
				call.Times(tt.wantPolls)
			}
			timeSource := clock.NewMockedTimeSource()
			scope := tally.NewTestScope("", nil)
			checker := newTestConvergenceChecker(t, client, timeSource, scope)

			var err error
			runUntilDone(timeSource, func() { err = checker.await(context.Background(), phaseScaleUp) })

			assert.NoError(t, err)
			converged, timeouts := convergenceOutcome(scope)
			assert.Equal(t, tt.wantConverged, converged)
			assert.Equal(t, tt.wantTimeoutCount, timeouts)
		})
	}
}

func TestConvergenceCheckerAwaitPollErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := sharddistributor.NewMockClient(ctrl)
	// A failed poll does not move the revision forward
	client.EXPECT().GetNamespaceStateIfNewer(gomock.Any(), &types.GetNamespaceStateIfNewerRequest{Namespace: testNamespace}).
		Return(nil, errors.New("unavailable")).MinTimes(2)
	timeSource := clock.NewMockedTimeSource()
	scope := tally.NewTestScope("", nil)
	checker := newTestConvergenceChecker(t, client, timeSource, scope)

	var err error
	runUntilDone(timeSource, func() { err = checker.await(context.Background(), phaseScaleUp) })

	assert.NoError(t, err)
	converged, timeouts := convergenceOutcome(scope)
	assert.False(t, converged)
	assert.Equal(t, int64(1), timeouts, "poll errors must count as mismatches")
}

func TestConvergenceCheckerAwaitShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctrl := gomock.NewController(t)
	client := sharddistributor.NewMockClient(ctrl)
	client.EXPECT().GetNamespaceStateIfNewer(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *types.GetNamespaceStateIfNewerRequest, ...yarpc.CallOption) (*types.GetNamespaceStateIfNewerResponse, error) {
			cancel()
			return staleState, nil
		},
	)
	timeSource := clock.NewMockedTimeSource()
	scope := tally.NewTestScope("", nil)
	checker := newTestConvergenceChecker(t, client, timeSource, scope)

	var err error
	runUntilDone(timeSource, func() { err = checker.await(ctx, phaseScaleUp) })

	assert.ErrorIs(t, err, context.Canceled)
	converged, timeouts := convergenceOutcome(scope)
	assert.False(t, converged)
	assert.Zero(t, timeouts)
}

func newTestConvergenceChecker(t *testing.T, client sharddistributor.Client, timeSource clock.TimeSource, scope tally.Scope) *convergenceChecker {
	return &convergenceChecker{
		client:    client,
		namespace: testNamespace,
		holders: func() []shardHolder {
			return []shardHolder{{shardKey: "1", executorID: "executor-a"}, {shardKey: "2", executorID: "executor-b"}}
		},
		timeout:    10 * time.Second,
		logger:     zaptest.NewLogger(t),
		timeSource: timeSource,
		scope:      scope,
	}
}

// convergenceOutcome reports whether a scale-up convergence was recorded and how many timed out.
func convergenceOutcome(scope tally.TestScope) (converged bool, timeouts int64) {
	snapshot := scope.Snapshot()
	tags := "+phase=" + phaseScaleUp
	_, converged = snapshot.Histograms()[canarymetrics.CanaryLoadConvergenceLatency+tags]
	if counter, ok := snapshot.Counters()[canarymetrics.CanaryLoadConvergenceTimeout+tags]; ok {
		timeouts = counter.Value()
	}
	return converged, timeouts
}

// namespaceState returns a response carrying a state that assigns each shard to its owner.
func namespaceState(revision int64, owners map[string]string) *types.GetNamespaceStateIfNewerResponse {
	assignments := make(map[string]*types.ExecutorShardAssignment)
	state := &types.WatchNamespaceStateResponse{AssignmentRevision: revision}
	for shardKey, executorID := range owners {
		assignment, ok := assignments[executorID]
		if !ok {
			assignment = &types.ExecutorShardAssignment{ExecutorID: executorID}
			assignments[executorID] = assignment
			state.Executors = append(state.Executors, assignment)
		}
		assignment.AssignedShards = append(assignment.AssignedShards, &types.Shard{ShardKey: shardKey})
	}
	return &types.GetNamespaceStateIfNewerResponse{AssignmentRevision: revision, State: state}
}

// drainedState is a stale state with the switcher shard drained.
func drainedState() *types.GetNamespaceStateIfNewerResponse {
	resp := namespaceState(2, map[string]string{"1": "executor-old", "2": "executor-b"})
	resp.State.DrainedShardKeys = []string{switcherShardID}
	return resp
}
