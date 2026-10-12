package namespaces

import (
	"fmt"
	"slices"

	"go.uber.org/fx"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
)

// Module provides the namespace registry to the fx application.
var Module = fx.Module("namespaces",
	fx.Provide(NewRegistry),
)

// Registry is the set of namespaces served by shard-manager.
// It is built once from static configuration and is immutable afterwards,
// so it is safe for concurrent use.
type Registry struct {
	list   []Namespace
	byName map[string]Namespace
}

// NewRegistry builds a registry from the namespaces in the static configuration.
// It returns an error if any namespace is invalid or a name appears more than once.
func NewRegistry(cfg config.ShardDistribution) (*Registry, error) {
	r := &Registry{
		list:   make([]Namespace, 0, len(cfg.Namespaces)),
		byName: make(map[string]Namespace, len(cfg.Namespaces)),
	}
	for _, nsCfg := range cfg.Namespaces {
		ns, err := fromConfig(nsCfg)
		if err != nil {
			return nil, fmt.Errorf("namespace %q: %w", nsCfg.Name, err)
		}
		if _, exists := r.byName[ns.Name]; exists {
			return nil, fmt.Errorf("namespace %q: configured more than once", ns.Name)
		}
		r.list = append(r.list, ns)
		r.byName[ns.Name] = ns
	}
	return r, nil
}

// MustNewRegistry is like NewRegistry but panics if the configuration is invalid.
func MustNewRegistry(cfg config.ShardDistribution) *Registry {
	r, err := NewRegistry(cfg)
	if err != nil {
		panic(err)
	}
	return r
}

// Get returns the namespace with the given name, or a *types.NamespaceNotFoundError.
func (r *Registry) Get(name string) (Namespace, error) {
	ns, ok := r.byName[name]
	if !ok {
		return Namespace{}, &types.NamespaceNotFoundError{Namespace: name}
	}
	return ns, nil
}

// List returns all namespaces in configuration order.
func (r *Registry) List() []Namespace {
	return slices.Clone(r.list)
}
