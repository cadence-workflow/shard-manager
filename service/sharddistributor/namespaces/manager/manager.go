package manager

import (
	"fmt"

	"go.uber.org/fx"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/cache"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/ephemeralassigner"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/namespaces"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// Module provides the namespace manager to the fx application.
var Module = fx.Module("namespace-manager",
	fx.Provide(New),
)

// Manager owns every configured namespace, so the components themselves never
// track which namespaces exist.
type Manager struct {
	namespaces []Namespace
	byName     map[string]Namespace
	logger     log.Logger
}

// Params are the dependencies shared by all namespaces.
type Params struct {
	fx.In

	Registry      *namespaces.Registry
	Store         store.Store
	Config        *config.Config
	Logger        log.Logger
	TimeSource    clock.TimeSource
	MetricsClient metrics.Client
	Lifecycle     fx.Lifecycle
}

// New builds every namespace in the registry and ties their start and stop to
// the lifecycle.
func New(p Params) *Manager {
	var namespaces []Namespace
	for _, nsCfg := range p.Registry.List() {
		shardCache := cache.NewShardCache(nsCfg.Name, p.Store, p.Logger)

		var assigner *ephemeralassigner.Assigner
		if nsCfg.Type == config.NamespaceTypeEphemeral {
			assigner = ephemeralassigner.New(nsCfg.Name, p.TimeSource, p.Config, p.Store, shardCache, p.MetricsClient)
		}

		namespaces = append(namespaces, newNamespace(nsCfg, shardCache, assigner))
	}

	m := newManager(p.Logger, namespaces)
	p.Lifecycle.Append(fx.StartStopHook(m.Start, m.Stop))
	return m
}

// Start starts every namespace in order. If one fails to start, the namespaces
// started before it are stopped again.
func (m *Manager) Start() error {
	for i, ns := range m.namespaces {
		if err := m.startNamespace(ns); err != nil {
			m.stopNamespaces(m.namespaces[:i])
			return err
		}
	}
	return nil
}

// Stop stops every namespace in reverse start order.
func (m *Manager) Stop() {
	m.stopNamespaces(m.namespaces)
}

func (m *Manager) stopNamespaces(nss []Namespace) {
	for i := len(nss) - 1; i >= 0; i-- {
		m.stopNamespace(nss[i])
	}
}

// startNamespace starts the cache first, then the assigner that reads from it.
func (m *Manager) startNamespace(ns Namespace) error {
	logger := m.logger.WithTags(tag.ShardNamespace(ns.Name()))
	logger.Info("Namespace state changed", tag.LifeCycleStarting)
	if err := ns.Cache().Start(); err != nil {
		logger.Error("Namespace state changed", tag.LifeCycleStartFailed, tag.Error(err))
		return fmt.Errorf("start cache of namespace %q: %w", ns.Name(), err)
	}
	if assigner := ns.Assigner(); assigner != nil {
		assigner.Start()
	}
	logger.Info("Namespace state changed", tag.LifeCycleStarted)
	return nil
}

// stopNamespace stops the assigner before the cache it reads from.
func (m *Manager) stopNamespace(ns Namespace) {
	logger := m.logger.WithTags(tag.ShardNamespace(ns.Name()))
	logger.Info("Namespace state changed", tag.LifeCycleStopping)
	if assigner := ns.Assigner(); assigner != nil {
		assigner.Stop()
	}
	ns.Cache().Stop()
	logger.Info("Namespace state changed", tag.LifeCycleStopped)
}

// NewTestManager builds a Manager over already-constructed namespaces, such as
// mocks, and does not manage their lifecycle. It is meant for tests only.
func NewTestManager(nss ...Namespace) *Manager {
	return newManager(log.NewNoop(), nss)
}

func newManager(logger log.Logger, nss []Namespace) *Manager {
	m := &Manager{
		namespaces: nss,
		byName:     make(map[string]Namespace, len(nss)),
		logger:     logger,
	}
	for _, ns := range nss {
		m.byName[ns.Name()] = ns
	}
	return m
}

// Get returns the namespace with the given name, or a *types.NamespaceNotFoundError.
func (m *Manager) Get(name string) (Namespace, error) {
	ns, ok := m.byName[name]
	if !ok {
		return nil, &types.NamespaceNotFoundError{Namespace: name}
	}
	return ns, nil
}

// List returns all namespaces in the order the manager was built with
// (sorted by name when built by New).
func (m *Manager) List() []Namespace {
	return append([]Namespace(nil), m.namespaces...)
}
