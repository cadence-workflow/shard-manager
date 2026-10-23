package manager

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination=namespace_mock.go Namespace

import (
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/cache"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/ephemeralassigner"
)

// Namespace is one configured namespace: its configuration and the components
// that serve it.
type Namespace interface {
	Name() string
	Config() config.Namespace
	Cache() cache.ShardCache
	// Assigner returns the ephemeral shard assigner, or nil if the namespace is not ephemeral.
	Assigner() *ephemeralassigner.Assigner
}

type namespace struct {
	config   config.Namespace
	cache    cache.ShardCache
	assigner *ephemeralassigner.Assigner
}

var _ Namespace = (*namespace)(nil)

// newNamespace bundles the components of one namespace. assigner is nil for
// namespaces that are not ephemeral.
func newNamespace(cfg config.Namespace, shardCache cache.ShardCache, assigner *ephemeralassigner.Assigner) *namespace {
	return &namespace{config: cfg, cache: shardCache, assigner: assigner}
}

func (n *namespace) Name() string                          { return n.config.Name }
func (n *namespace) Config() config.Namespace              { return n.config }
func (n *namespace) Cache() cache.ShardCache               { return n.cache }
func (n *namespace) Assigner() *ephemeralassigner.Assigner { return n.assigner }
