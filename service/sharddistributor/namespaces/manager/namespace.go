package manager

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination=namespace_mock.go Namespace

import (
	"github.com/cadence-workflow/shard-manager/common/dynamicconfig/dynamicproperties"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/cache"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
)

// Namespace is one configured namespace: its configuration and the components
// that serve it.
type Namespace interface {
	Name() string
	// Config returns the configuration the namespace runs with: the static
	// configuration with dynamic config applied by config.EffectiveNamespace.
	Config() config.Namespace
	Cache() cache.ShardCache
}

type namespace struct {
	staticConfig  config.Namespace
	fixedShardNum dynamicproperties.IntPropertyFnWithNamespaceFilters
	cache         cache.ShardCache
}

var _ Namespace = (*namespace)(nil)

// newNamespace bundles the components of one namespace.
func newNamespace(staticConfig config.Namespace, fixedShardNum dynamicproperties.IntPropertyFnWithNamespaceFilters, shardCache cache.ShardCache) *namespace {
	return &namespace{staticConfig: staticConfig, fixedShardNum: fixedShardNum, cache: shardCache}
}

func (n *namespace) Name() string { return n.staticConfig.Name }
func (n *namespace) Config() config.Namespace {
	fixedShardNum := n.fixedShardNum(n.staticConfig.Name)
	return config.EffectiveNamespace(n.staticConfig, fixedShardNum)
}
func (n *namespace) Cache() cache.ShardCache { return n.cache }
