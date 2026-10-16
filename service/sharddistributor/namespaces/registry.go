package namespaces

import (
	"errors"
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
	list   []config.Namespace
	byName map[string]config.Namespace
}

// NewRegistry builds a registry from the namespaces in the static configuration.
// It returns an error if any namespace is invalid or a name appears more than once.
func NewRegistry(cfg config.ShardDistribution) (*Registry, error) {
	r := &Registry{
		list:   slices.Clone(cfg.Namespaces),
		byName: make(map[string]config.Namespace, len(cfg.Namespaces)),
	}
	for _, ns := range cfg.Namespaces {
		if err := validate(ns); err != nil {
			return nil, fmt.Errorf("namespace %q: %w", ns.Name, err)
		}
		if _, exists := r.byName[ns.Name]; exists {
			return nil, fmt.Errorf("namespace %q: configured more than once", ns.Name)
		}
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

func validate(ns config.Namespace) error {
	if ns.Name == "" {
		return errors.New("name must not be empty")
	}
	switch ns.Type {
	case config.NamespaceTypeFixed:
		if ns.ShardNum <= 0 {
			return fmt.Errorf("fixed namespace must have a positive shardNum, got %d", ns.ShardNum)
		}
	case config.NamespaceTypeEphemeral:
	default:
		return fmt.Errorf("unknown type %q, must be %q or %q", ns.Type, config.NamespaceTypeFixed, config.NamespaceTypeEphemeral)
	}
	return nil
}

// Get returns the namespace with the given name, or a *types.NamespaceNotFoundError.
func (r *Registry) Get(name string) (config.Namespace, error) {
	ns, ok := r.byName[name]
	if !ok {
		return config.Namespace{}, &types.NamespaceNotFoundError{Namespace: name}
	}
	return ns, nil
}

// List returns all namespaces in configuration order.
func (r *Registry) List() []config.Namespace {
	return slices.Clone(r.list)
}
