package executorstore

import (
	"testing"

	"github.com/stretchr/testify/require"
	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdclient"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/testhelper"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/storetest"
)

func TestStoreSuite(t *testing.T) {
	storetest.Run(t, storetest.Backend{
		NewStore: func(t *testing.T) store.Store {
			tc := testhelper.SetupStoreTestCluster(t)
			etcdConfig, err := etcdclient.NewExecutorStoreConfig(tc.SDConfig)
			require.NoError(t, err)

			watcher := newNamespaceWatcher(tc.Client, etcdConfig, testlogger.New(t), clock.NewRealTimeSource(), metrics.NewNoopMetricsClient())
			t.Cleanup(watcher.Stop)
			return newEtcdStore(createStore(t, tc), watcher)
		},
		FailingGuard: func() store.GuardFunc {
			return func(txn store.Txn) (store.Txn, error) {
				// A key that is never written has version 0, so the compare never holds.
				return txn.(clientv3.Txn).If(clientv3.Compare(clientv3.Version("/storetest/never-written"), ">", 0)), nil
			}
		},
	})
}
