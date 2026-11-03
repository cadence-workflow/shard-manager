package manager

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination=namespace_mock.go Namespace

import (
	"github.com/cadence-workflow/shard-manager/common/dynamicconfig/dynamicproperties"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/cache"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/ephemeralassigner"
)

// Namespace is one configured namespace: its configuration and the components
// that serve it.
type Namespace interface {
	Name() string
	// Config returns the configuration the namespace runs with: the static
	// configuration with dynamic config applied by config.EffectiveNamespace.
	Config() config.Namespace
	Cache() cache.ShardCache
	// Assigner returns the ephemeral shard assigner, or nil if the namespace is
	// fixed in the static configuration. It is never nil while Config is ephemeral.
	Assigner() *ephemeralassigner.Assigner
}

type namespace struct {
	staticConfig  config.Namespace
	fixedShardNum dynamicproperties.IntPropertyFnWithNamespaceFilters
	cache         cache.ShardCache
	assigner      *ephemeralassigner.Assigner
}

var _ Namespace = (*namespace)(nil)

// newNamespace bundles the components of one namespace. assigner is nil for
// namespaces that are fixed in the static configuration.
func newNamespace(staticConfig config.Namespace, fixedShardNum dynamicproperties.IntPropertyFnWithNamespaceFilters, shardCache cache.ShardCache, assigner *ephemeralassigner.Assigner) *namespace {
	return &namespace{staticConfig: staticConfig, fixedShardNum: fixedShardNum, cache: shardCache, assigner: assigner}
}

func (n *namespace) Name() string { return n.staticConfig.Name }
func (n *namespace) Config() config.Namespace {
	fixedShardNum := n.fixedShardNum(n.staticConfig.Name)
	return config.EffectiveNamespace(n.staticConfig, fixedShardNum)
}
func (n *namespace) Cache() cache.ShardCache               { return n.cache }
func (n *namespace) Assigner() *ephemeralassigner.Assigner { return n.assigner }
