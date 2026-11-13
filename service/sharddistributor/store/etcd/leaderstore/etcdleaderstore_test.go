package leaderstore

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdclient"
	"github.com/cadence-workflow/shard-manager/testflags"
)

const _testTerm = 2 * time.Second

// TestTermExpiresWithoutRenewal checks that etcd ends a term the holder never touches again,
// and that the holder's writes are fenced once another instance takes over.
func TestTermExpiresWithoutRenewal(t *testing.T) {
	tc := setupETCDCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stalled := tc.newElection(ctx, t)
	stalledTerm, err := stalled.Campaign(ctx, "stalled-host")
	require.NoError(t, err)
	assert.True(t, tc.guardedPut(ctx, t, stalled.Guard()))

	start := time.Now()
	healthy := tc.newElection(ctx, t)
	_, err = healthy.Campaign(ctx, "healthy-host")
	require.NoError(t, err)
	assert.Greater(t, time.Since(start), _testTerm/2, "takeover should wait for lease expiry")
	assert.Less(t, time.Since(start), _testTerm+time.Second, "takeover should follow lease expiry")

	assert.ErrorIs(t, stalledTerm.Err(), context.DeadlineExceeded, "the stalled term should have ended before the takeover")
	assert.False(t, tc.guardedPut(ctx, t, stalled.Guard()), "a stale holder must not commit")
	assert.True(t, tc.guardedPut(ctx, t, healthy.Guard()))
}

func TestCampaignWaitsForLeader(t *testing.T) {
	t.Run("resign hands over before the term ends", func(t *testing.T) {
		tc, ctx := setupETCDCluster(t), t.Context()
		leader := tc.newElection(ctx, t)
		_, err := leader.Campaign(ctx, "leader")
		require.NoError(t, err)

		tc.campaignAfter(ctx, t, func() { require.NoError(t, leader.Resign(ctx)) })
	})
	t.Run("key from the previous election code blocks until deleted", func(t *testing.T) {
		tc, ctx := setupETCDCluster(t), t.Context()
		key := fmt.Sprintf("%s/%s/leader/694d7f3a", tc.storeConfig.Prefix, _testNamespace)
		_, err := tc.client.Put(ctx, key, "old-host")
		require.NoError(t, err)

		tc.campaignAfter(ctx, t, func() {
			_, err := tc.client.Delete(ctx, key)
			require.NoError(t, err)
		})
	})
}

// campaignAfter checks that a campaign waits for release and wins well before the term ends.
func (tc *testCluster) campaignAfter(ctx context.Context, t *testing.T, release func()) {
	contender := tc.newElection(ctx, t)
	campaignCtx, cancel := context.WithTimeout(ctx, _testTerm/2)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := contender.Campaign(campaignCtx, "contender")
		result <- err
	}()

	time.Sleep(100 * time.Millisecond)
	require.Empty(t, result, "campaign should wait for release")
	release()
	assert.NoError(t, <-result)
}

const _testNamespace = "test-namespace"

func (tc *testCluster) newElection(ctx context.Context, t *testing.T) store.Election {
	election, err := tc.store.CreateElection(ctx, _testNamespace)
	require.NoError(t, err)
	return election
}

func (tc *testCluster) guardedPut(ctx context.Context, t *testing.T, guard store.GuardFunc) bool {
	txn, err := guard(tc.client.Txn(ctx))
	require.NoError(t, err)
	resp, err := txn.(clientv3.Txn).Then(clientv3.OpPut(tc.storeConfig.Prefix+"/data", "value")).Commit()
	require.NoError(t, err)
	return resp.Succeeded
}

// testCluster represents a test etcd cluster with its resources
type testCluster struct {
	store       store.Elector
	storeConfig etcdclient.LeaderStoreConfig
	client      *clientv3.Client
}

// setupETCDCluster initializes an etcd cluster for testing
func setupETCDCluster(t *testing.T) *testCluster {
	t.Helper()

	flag.Parse()
	testflags.RequireEtcd(t)

	endpoints := strings.Split(os.Getenv("ETCD_ENDPOINTS"), ",")
	if len(endpoints) == 0 || endpoints[0] == "" {
		// default etcd port
		endpoints = []string{"localhost:2379"}
	}

	t.Logf("ETCD endpoints: %v", endpoints)

	testConfig := etcdclient.LeaderStoreConfig{
		BaseConfig: etcdclient.BaseConfig{
			Endpoints:   endpoints,
			DialTimeout: 5 * time.Second,
			Prefix:      fmt.Sprintf("/election/%s", t.Name()),
		},
	}

	// Create etcd client
	rawClient, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { rawClient.Close() })
	_, err = rawClient.Delete(context.Background(), testConfig.Prefix, clientv3.WithPrefix())
	require.NoError(t, err)

	// Create store
	storeParams := StoreParams{
		Client:            etcdclient.NewClient(rawClient),
		Cfg:               testConfig,
		ShardDistribution: config.ShardDistribution{Process: config.LeaderProcess{Period: _testTerm}},
	}

	store, err := NewLeaderStore(storeParams)
	require.NoError(t, err)

	return &testCluster{
		store:       store,
		storeConfig: testConfig,
		client:      rawClient,
	}
}
