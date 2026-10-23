package manager

import (
	"fmt"

	"go.uber.org/fx"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
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

	m := newManager(namespaces)
	p.Lifecycle.Append(fx.StartStopHook(m.Start, m.Stop))
	return m
}

// Start starts every namespace: its cache first, then the assigner that reads
// from it. If a cache fails to start, everything is stopped again; Stop is safe
// on components that were never started.
func (m *Manager) Start() error {
	for _, ns := range m.namespaces {
		if err := ns.Cache().Start(); err != nil {
			m.Stop()
			return fmt.Errorf("start cache of namespace %q: %w", ns.Name(), err)
		}
		if assigner := ns.Assigner(); assigner != nil {
			assigner.Start()
		}
	}
	return nil
}

// Stop stops every namespace in reverse order, each assigner before the cache
// it reads from.
func (m *Manager) Stop() {
	for i := len(m.namespaces) - 1; i >= 0; i-- {
		ns := m.namespaces[i]
		if assigner := ns.Assigner(); assigner != nil {
			assigner.Stop()
		}
		ns.Cache().Stop()
	}
}

// NewTestManager builds a Manager over already-constructed namespaces, such as
// mocks, and does not manage their lifecycle. It is meant for tests only.
func NewTestManager(nss ...Namespace) *Manager {
	return newManager(nss)
}

func newManager(nss []Namespace) *Manager {
	m := &Manager{
		namespaces: nss,
		byName:     make(map[string]Namespace, len(nss)),
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
