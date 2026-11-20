package election

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/leader/process"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

const (
	_testHost = "localhost"
)

var (
	_testNamespace = config.Namespace{Name: "test-namespace"}
)

var (
	_testFailedElectionCooldown = 10 * time.Second
)

// campaignForever wins a term that only ends when the elector's context is cancelled.
func campaignForever(ctx context.Context, _ string) (context.Context, error) {
	return ctx, nil
}

func TestElector_Run(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctrl := gomock.NewController(t)
	logger := testlogger.New(t)
	timeSource := clock.NewMockedTimeSource()

	election := store.NewMockElection(ctrl)
	election.EXPECT().Campaign(gomock.Any(), _testHost).DoAndReturn(campaignForever)

	finished := make(chan struct{})
	// once test is done resign will be called
	election.EXPECT().Resign(gomock.Any()).DoAndReturn(func(_ context.Context) error {
		close(finished)
		return nil
	})

	leaderStore := store.NewMockElector(ctrl)
	leaderStore.EXPECT().CreateElection(gomock.Any(), _testNamespace.Name).Return(election, nil)

	shardStore := store.NewMockStore(ctrl)

	processFactory := process.NewMockFactory(ctrl)
	processRunner := process.NewMockProcessor(ctrl)
	processFactory.EXPECT().CreateProcessor(_testNamespace, shardStore, election).Return(processRunner)

	factory := NewElectionFactory(FactoryParams{
		HostName: _testHost,
		Cfg: config.ShardDistribution{
			Election: config.Election{
				FailedElectionCooldown: _testFailedElectionCooldown,
			},
		},
		LeaderStore:    leaderStore,
		Store:          shardStore,
		Logger:         logger,
		Clock:          timeSource,
		ProcessFactory: processFactory,
	})

	el, err := factory.CreateElector(context.Background(), _testNamespace)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Track callback executions
	onLeaderCalled := false
	onResignCalled := false

	processRunner.EXPECT().Run(gomock.Any()).DoAndReturn(func(_ context.Context) error {
		onLeaderCalled = true
		return nil
	})

	processRunner.EXPECT().Terminate(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
		onResignCalled = true
		return nil
	})

	leaderChan := el.Run(ctx)
	assert.True(t, <-leaderChan)
	assert.True(t, onLeaderCalled, "OnLeader callback should have been called")
	assert.False(t, onResignCalled, "OnResign callback should not have been called")
	cancel()
	<-finished
	// Drain leaderChan until it is closed: the goroutine sends false and then
	// closes the channel only after resign() has returned,
	// so this guarantees all mock expectations are satisfied before the test exits.
	for range leaderChan {
	}
}

func TestElector_Run_Resign(t *testing.T) {
	defer goleak.VerifyNone(t)
	t.Run("context_canceled", func(t *testing.T) {
		leaderChan, p := prepareRun(t, nil, nil)
		p.election.EXPECT().Resign(gomock.Any()).Return(nil)
		p.cancel()
		assert.False(t, <-leaderChan)
		// Wait for the goroutine to exit
		for range leaderChan {
		}
	})
	t.Run("term_ended", func(t *testing.T) {
		// Verify onResign is called before resignation
		onResignCalled := false
		leaderChan, p := prepareRun(t, nil, func(ctx context.Context) error {
			onResignCalled = true
			return nil
		})

		p.election.EXPECT().Resign(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
			assert.True(t, onResignCalled, "OnResign callback should be called before Resign")
			return nil
		})

		p.endTerm()
		// The next term starts straight away.
		p.timeSource.BlockUntil(1)
		assert.False(t, <-leaderChan)
		p.cancel()
		// Wait for the goroutine to exit
		for range leaderChan {
		}
	})

	t.Run("onResign_error", func(t *testing.T) {
		// Set onResign to return an error
		onResignCalled := false
		resignErr := errors.New("resign error")
		onResign := func(ctx context.Context) error {
			onResignCalled = true
			return resignErr
		}

		leaderChan, p := prepareRun(t, nil, onResign)
		p.election.EXPECT().Resign(gomock.Any()).Return(nil)
		p.endTerm()
		p.timeSource.BlockUntil(1)
		assert.False(t, <-leaderChan)
		assert.True(t, onResignCalled, "OnResign callback should have been called")

		p.cancel()
		// Wait for the goroutine to exit
		for range leaderChan {
		}
	})
	t.Run("OnResign_and_resign_error", func(t *testing.T) {
		// Set onResign to return an error
		onResignCalled := false
		resignErr := errors.New("resign error")
		onResign := func(ctx context.Context) error {
			onResignCalled = true
			return resignErr
		}

		leaderChan, p := prepareRun(t, nil, onResign)
		p.election.EXPECT().Resign(gomock.Any()).Return(fmt.Errorf("failed to resign"))
		p.endTerm()
		p.timeSource.BlockUntil(1)
		assert.False(t, <-leaderChan)
		assert.True(t, onResignCalled, "OnResign callback should have been called")

		p.cancel()
		// Wait for the goroutine to exit
		for range leaderChan {
		}
	})
}

type runParams struct {
	ctx        context.Context
	cancel     context.CancelFunc
	timeSource clock.MockedTimeSource
	endTerm    context.CancelFunc
	election   *store.MockElection
	onLeader   ProcessFunc
	onResign   ProcessFunc
}

func prepareRun(t *testing.T, onLeader, onResign ProcessFunc) (<-chan bool, runParams) {
	ctrl := gomock.NewController(t)
	logger := testlogger.New(t)
	timeSource := clock.NewMockedTimeSource()

	var endTerm context.CancelFunc
	election := store.NewMockElection(ctrl)
	election.EXPECT().Campaign(gomock.Any(), _testHost).DoAndReturn(func(ctx context.Context, _ string) (context.Context, error) {
		termCtx, cancel := context.WithCancel(ctx)
		endTerm = cancel
		return termCtx, nil
	})

	leaderStore := store.NewMockElector(ctrl)
	leaderStore.EXPECT().CreateElection(gomock.Any(), _testNamespace.Name).Return(election, nil)
	// The next term fails to start, so the elector waits out the cooldown.
	leaderStore.EXPECT().CreateElection(gomock.Any(), _testNamespace.Name).Return(nil, errors.New("next term")).AnyTimes()

	shardStore := store.NewMockStore(ctrl)

	processFactory := process.NewMockFactory(ctrl)
	processRunner := process.NewMockProcessor(ctrl)
	processFactory.EXPECT().CreateProcessor(_testNamespace, shardStore, election).Return(processRunner)

	factory := NewElectionFactory(FactoryParams{
		HostName: _testHost,
		Cfg: config.ShardDistribution{
			Election: config.Election{
				FailedElectionCooldown: _testFailedElectionCooldown,
			},
		},
		LeaderStore:    leaderStore,
		Store:          shardStore,
		Logger:         logger,
		Clock:          timeSource,
		ProcessFactory: processFactory,
	})

	elector, err := factory.CreateElector(context.Background(), _testNamespace)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	// Default callbacks
	if onLeader == nil {
		onLeader = func(ctx context.Context) error {
			return nil
		}
	}

	if onResign == nil {
		onResign = func(ctx context.Context) error {
			return nil
		}
	}

	processRunner.EXPECT().Run(gomock.Any()).DoAndReturn(onLeader)
	processRunner.EXPECT().Terminate(gomock.Any()).DoAndReturn(onResign)

	leaderChan := elector.Run(ctx)
	assert.True(t, <-leaderChan)

	return leaderChan, runParams{
		ctx:        ctx,
		cancel:     cancel,
		timeSource: timeSource,
		endTerm:    endTerm,
		election:   election,
		onLeader:   onLeader,
		onResign:   onResign,
	}
}

func TestOnLeader_Error(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctrl := gomock.NewController(t)
	logger := testlogger.New(t)
	timeSource := clock.NewMockedTimeSource()

	election := store.NewMockElection(ctrl)
	election.EXPECT().Campaign(gomock.Any(), _testHost).DoAndReturn(campaignForever)
	// Expect resignation after onLeader failure
	election.EXPECT().Resign(gomock.Any()).Return(nil)

	leaderStore := store.NewMockElector(ctrl)
	leaderStore.EXPECT().CreateElection(gomock.Any(), _testNamespace.Name).Return(election, nil)

	shardStore := store.NewMockStore(ctrl)

	processFactory := process.NewMockFactory(ctrl)
	processRunner := process.NewMockProcessor(ctrl)
	processFactory.EXPECT().CreateProcessor(_testNamespace, shardStore, election).Return(processRunner)

	// Create elector directly for test control
	el := &elector{
		namespace:   _testNamespace,
		leaderStore: leaderStore,
		store:       shardStore,
		logger:      logger,
		cfg: config.Election{
			FailedElectionCooldown: _testFailedElectionCooldown,
		},
		clock:          timeSource,
		hostname:       _testHost,
		processFactory: processFactory,
	}

	// Create a cancelable context for the test
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel() // Ensure context is canceled at the end of the test

	// Make onLeader return an error
	onLeaderErr := errors.New("leader error")
	processRunner.EXPECT().Run(gomock.Any()).Return(onLeaderErr)
	processRunner.EXPECT().Terminate(gomock.Any()).Return(nil)

	// Run the test
	leaderCh := make(chan bool, 1)
	err := el.runElection(ctx, leaderCh)

	// Error should contain our onLeader error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "onLeader")
	assert.Contains(t, err.Error(), "leader error")
}
