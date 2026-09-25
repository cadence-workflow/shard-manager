package namespace

import "github.com/cadence-workflow/shard-manager/service/sharddistributor/config"

// Registry is an indexed view of the static namespace configuration,
// providing O(1) lookup by name and preserving declaration order.
type Registry struct {
	namespaces map[string]config.Namespace
	ordered    []config.Namespace
}

// NewRegistry builds a Registry from the configured namespace list.
func NewRegistry(namespaces []config.Namespace) Registry {
	m := make(map[string]config.Namespace, len(namespaces))
	for _, ns := range namespaces {
		m[ns.Name] = ns
	}
	return Registry{
		namespaces: m,
		ordered:    namespaces,
	}
}

// Get returns the Namespace config for the given name, and false if not found.
func (r Registry) Get(name string) (config.Namespace, bool) {
	ns, ok := r.namespaces[name]
	return ns, ok
}

// All returns the namespaces in their original declaration order.
func (r Registry) All() []config.Namespace {
	return r.ordered
}
