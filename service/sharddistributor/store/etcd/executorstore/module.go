package executorstore

import (
	"go.uber.org/fx"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdclient"
)

// Module builds the etcd operations and the namespace watch separately and composes
// them into the store.Store the rest of the application consumes.
var Module = fx.Module("executorstore",
	fx.Provide(provideExecutorStoreImpl),
	fx.Provide(provideNamespaceWatcher),
	fx.Provide(newEtcdStore),
)

// etcdStore is the store.Store implementation: stateless etcd operations plus the
// namespace watch, which share no state and are composed rather than entangled.
type etcdStore struct {
	*executorStoreImpl
	*namespaceWatcher
}

func newEtcdStore(impl *executorStoreImpl, watcher *namespaceWatcher) store.Store {
	return &etcdStore{executorStoreImpl: impl, namespaceWatcher: watcher}
}

// ExecutorStoreParams defines the dependencies for the etcd operations, for use with fx.
type ExecutorStoreParams struct {
	fx.In

	Client        etcdclient.Client `name:"executorstore"`
	ETCDConfig    etcdclient.ExecutorStoreConfig
	Logger        log.Logger
	TimeSource    clock.TimeSource
	Config        *config.Config
	MetricsClient metrics.Client
}

func provideExecutorStoreImpl(p ExecutorStoreParams) (*executorStoreImpl, error) {
	return newExecutorStoreImpl(p.Client, p.ETCDConfig, p.Logger, p.TimeSource, p.Config, p.MetricsClient)
}

// NamespaceWatcherParams defines the dependencies for the namespace watcher, for use with fx.
type NamespaceWatcherParams struct {
	fx.In

	Client        etcdclient.Client `name:"executorstore"`
	ETCDConfig    etcdclient.ExecutorStoreConfig
	Lifecycle     fx.Lifecycle
	Logger        log.Logger
	TimeSource    clock.TimeSource
	MetricsClient metrics.Client
}

// provideNamespaceWatcher registers the watcher's shutdown with the lifecycle: there is
// nothing to start, since a watch only exists once something subscribes.
func provideNamespaceWatcher(p NamespaceWatcherParams) *namespaceWatcher {
	watcher := newNamespaceWatcher(p.Client, p.ETCDConfig, p.Logger, p.TimeSource, p.MetricsClient)
	p.Lifecycle.Append(fx.StopHook(watcher.Stop))

	return watcher
}
