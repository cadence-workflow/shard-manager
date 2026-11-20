package election

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/fx"
	"go.uber.org/multierr"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/leader/process"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination=election_mock.go Factory,Elector

var errSelfResign = fmt.Errorf("self-resigned")

// Module provides election factory for fx app.
var Module = fx.Module(
	"leader-election",
	fx.Provide(NewElectionFactory),
)

type ProcessFunc func(ctx context.Context) error

// Elector handles leader election for a specific namespace
type Elector interface {
	Run(ctx context.Context) <-chan bool
}

// Factory creates elector instances
type Factory interface {
	CreateElector(ctx context.Context, namespaceCfg config.Namespace) (Elector, error)
}

type electionFactory struct {
	hostname       string
	cfg            config.Election
	leaderStore    store.Elector
	store          store.Store
	logger         log.Logger
	serviceID      string
	clock          clock.TimeSource
	processFactory process.Factory
}

type elector struct {
	hostname       string
	namespace      config.Namespace
	leaderStore    store.Elector
	store          store.Store
	logger         log.Logger
	cfg            config.Election
	leaderStarted  time.Time
	clock          clock.TimeSource
	processFactory process.Factory
}

type FactoryParams struct {
	fx.In

	HostName       string `name:"hostname"`
	Cfg            config.ShardDistribution
	LeaderStore    store.Elector
	Logger         log.Logger
	Clock          clock.TimeSource
	ProcessFactory process.Factory
	Store          store.Store
}

// NewElectionFactory creates a new election factory
func NewElectionFactory(p FactoryParams) Factory {
	return &electionFactory{
		cfg:            p.Cfg.Election,
		leaderStore:    p.LeaderStore,
		store:          p.Store,
		logger:         p.Logger,
		clock:          p.Clock,
		hostname:       p.HostName,
		processFactory: p.ProcessFactory,
	}
}

// CreateElector creates a new elector for the given namespace
func (f *electionFactory) CreateElector(ctx context.Context, namespaceCfg config.Namespace) (Elector, error) {
	return &elector{
		namespace:      namespaceCfg,
		leaderStore:    f.leaderStore,
		store:          f.store,
		logger:         f.logger.WithTags(tag.ComponentLeaderElection, tag.ShardNamespace(namespaceCfg.Name)),
		cfg:            f.cfg,
		clock:          f.clock,
		hostname:       f.hostname,
		processFactory: f.processFactory,
	}, nil
}

// Run starts the leader election process it returns a channel that will return the value if the current instance becomes the leader or resigns from leadership.
func (e *elector) Run(ctx context.Context) <-chan bool {
	leaderCh := make(chan bool, 1)

	// Create a child context that we can explicitly cancel when errors occur
	runCtx, cancelRun := context.WithCancel(ctx)

	go func() {
		defer close(leaderCh)
		defer cancelRun() // Ensure child context is canceled on exit
		defer func() {
			e.logger.Info("Leader election process exiting")
			if r := recover(); r != nil {
				e.logger.Error("Panic in election process", tag.Value(r))
			}
		}()

		for {
			if err := e.runElection(runCtx, leaderCh); err != nil {
				// Check if parent context is already canceled
				if runCtx.Err() != nil {
					e.logger.Info("Context canceled, stopping election loop", tag.Error(runCtx.Err()))
					return
				}

				// Self resign, immediately retry, otherwise, wait
				if !errors.Is(err, errSelfResign) {
					e.logger.Error("Error in election, retrying", tag.Error(err))

					select {
					case <-runCtx.Done():
						e.logger.Info("Context canceled, stopping election loop, exit immediately", tag.Error(runCtx.Err()))
						return // Context was canceled, exit immediately
					case <-e.clock.After(e.cfg.FailedElectionCooldown):
						e.logger.Info("Cooldown period ended, retrying election")
						// Continue after cooldown
					}
				}
			}
			if runCtx.Err() != nil {
				e.logger.Info("Context canceled, stopping election loop", tag.Error(runCtx.Err()))
				break
			}
		}
	}()

	return leaderCh
}

// runElection runs a single election attempt
func (e *elector) runElection(ctx context.Context, leaderCh chan<- bool) (err error) {
	e.logger.Debug("Creating election")
	election, err := e.leaderStore.CreateElection(ctx, e.namespace.Name)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	var leaderProcess process.Processor

	defer func() {
		resignErr := e.resign(election, leaderProcess)
		if resignErr != nil {
			if err == nil {
				err = resignErr
			} else {
				// Something already went wrong, so the process is most likely not started or stopped.
				// It should be safe to report leadership lost.
				e.logger.Error("Error resigning leader", tag.Error(resignErr))
			}
		}

		// We are no longer a leader and OnResign is called - notify the manager that leader elected process
		leaderCh <- false
	}()

	e.logger.Debug("Starting campaign for leader")
	// Campaign to become leader
	termCtx, err := election.Campaign(ctx, e.hostname)
	if err != nil {
		return fmt.Errorf("failed to campaign: %w", err)
	}

	leaderProcess = e.processFactory.CreateProcessor(e.namespace, e.store, election)

	e.logger.Debug("Run leader process")
	err = leaderProcess.Run(termCtx)
	if err != nil {
		return fmt.Errorf("onLeader: %w", err)
	}

	// Successfully became leader
	e.leaderStarted = e.clock.Now()
	leaderCh <- true

	e.logger.Debug("Became leader")

	// The term context ends when we lose leadership or on shutdown.
	<-termCtx.Done()
	if ctx.Err() != nil {
		e.logger.Info("Context cancelled while leader")
		return nil
	}
	e.logger.Debug("Leadership term ended")
	return errSelfResign
}

func (e *elector) resign(election store.Election, processor process.Processor) error {
	ctx, cancel := e.clock.ContextWithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var resignErr error

	if processor != nil {
		// First try to terminate the processor to stop leader processing
		if err := processor.Terminate(ctx); err != nil {
			resignErr = fmt.Errorf("terminate processor: %w", err)
		}
	}

	// Then try to resign leadership
	if err := election.Resign(ctx); err != nil {
		resignErr = multierr.Append(resignErr, fmt.Errorf("resign election: %w", err))
	}

	return resignErr
}
