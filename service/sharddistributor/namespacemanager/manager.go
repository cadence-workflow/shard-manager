package namespacemanager

import (
	"context"
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

// Module provides the namespace manager and ties its components to the fx lifecycle.
var Module = fx.Module("namespace-manager",
	fx.Provide(New),
)

// Namespace is the runtime of one configured namespace: its configuration and
// the components that serve it.
type Namespace struct {
	config   config.Namespace
	cache    cache.NamespaceCache
	assigner *ephemeralassigner.Assigner
}

// NewNamespace bundles the components of one namespace. assigner is nil for
// namespaces that are not ephemeral.
func NewNamespace(cfg config.Namespace, namespaceCache cache.NamespaceCache, assigner *ephemeralassigner.Assigner) *Namespace {
	return &Namespace{config: cfg, cache: namespaceCache, assigner: assigner}
}

func (n *Namespace) Name() string             { return n.config.Name }
func (n *Namespace) Config() config.Namespace { return n.config }

func (n *Namespace) Cache() cache.NamespaceCache { return n.cache }

// Assigner returns the ephemeral shard assigner, or nil if the namespace is not ephemeral.
func (n *Namespace) Assigner() *ephemeralassigner.Assigner { return n.assigner }

// component is a piece of a namespace runtime whose lifecycle the Manager owns.
type component struct {
	name  string
	start func() error
	stop  func()
}

// Manager owns the runtime of every configured namespace. It starts the
// components of all namespaces together and stops them in reverse order, so the
// components themselves never track which namespaces exist.
type Manager struct {
	namespaces []*Namespace
	byName     map[string]*Namespace
	components []component
}

// Params are the dependencies shared by all namespace runtimes.
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

// New builds the runtime of every namespace in the registry and registers
// their start and stop with the lifecycle.
func New(p Params) *Manager {
	var (
		runtimes   []*Namespace
		components []component
	)
	for _, nsCfg := range p.Registry.List() {
		shardCache := cache.NewShardCache(nsCfg.Name, p.Store, p.Logger, p.TimeSource, p.MetricsClient)
		components = append(components, component{
			name:  nsCfg.Name + "/cache",
			start: shardCache.Start,
			stop:  shardCache.Stop,
		})

		var assigner *ephemeralassigner.Assigner
		if nsCfg.Type == config.NamespaceTypeEphemeral {
			assigner = ephemeralassigner.New(nsCfg.Name, p.TimeSource, p.Config, p.Store, shardCache, p.MetricsClient)
			components = append(components, component{
				name:  nsCfg.Name + "/assigner",
				start: func() error { assigner.Start(); return nil },
				stop:  assigner.Stop,
			})
		}

		runtimes = append(runtimes, NewNamespace(nsCfg, shardCache, assigner))
	}

	m := newManager(runtimes)
	m.components = components
	p.Lifecycle.Append(fx.Hook{OnStart: m.start, OnStop: m.stop})
	return m
}

// NewStatic builds a Manager over already-constructed namespaces and does not
// manage their lifecycle.
func NewStatic(nss ...*Namespace) *Manager {
	return newManager(nss)
}

func newManager(nss []*Namespace) *Manager {
	m := &Manager{
		namespaces: nss,
		byName:     make(map[string]*Namespace, len(nss)),
	}
	for _, ns := range nss {
		m.byName[ns.Name()] = ns
	}
	return m
}

// Get returns the namespace with the given name, or a *types.NamespaceNotFoundError.
func (m *Manager) Get(name string) (*Namespace, error) {
	ns, ok := m.byName[name]
	if !ok {
		return nil, &types.NamespaceNotFoundError{Namespace: name}
	}
	return ns, nil
}

// List returns all namespaces in configuration order.
func (m *Manager) List() []*Namespace {
	return append([]*Namespace(nil), m.namespaces...)
}

// start starts the components in order. If one fails, the components already
// started are stopped before returning the error.
func (m *Manager) start(context.Context) error {
	for i, c := range m.components {
		if err := c.start(); err != nil {
			stopAll(m.components[:i])
			return fmt.Errorf("start %s: %w", c.name, err)
		}
	}
	return nil
}

func (m *Manager) stop(context.Context) error {
	stopAll(m.components)
	return nil
}

func stopAll(components []component) {
	for i := len(components) - 1; i >= 0; i-- {
		components[i].stop()
	}
}
