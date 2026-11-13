package leaderstore

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/fx"

	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdclient"
)

type LeaderStore struct {
	client etcdclient.Client
	config etcdclient.LeaderStoreConfig
	// term is one rebalance period, since every term starts with a rebalance.
	term time.Duration
}

// StoreParams defines the dependencies for the etcd store, for use with fx.
type StoreParams struct {
	fx.In

	Client            etcdclient.Client `name:"leaderstore"`
	Cfg               etcdclient.LeaderStoreConfig
	ShardDistribution config.ShardDistribution
	Logger            log.Logger
}

// NewLeaderStore creates a new leaderstore backed by ETCD.
func NewLeaderStore(p StoreParams) (store.Elector, error) {
	return &LeaderStore{
		client: p.Client,
		config: p.Cfg,
		term:   p.ShardDistribution.Process.Period,
	}, nil
}

func (ls *LeaderStore) CreateElection(ctx context.Context, namespace string) (store.Election, error) {
	return &election{
		client: ls.client,
		prefix: fmt.Sprintf("%s/%s/leader/", ls.config.Prefix, namespace),
		term:   ls.term,
	}, nil
}

// election is a single leadership term. Its lease is never renewed, so etcd ends the term
// even if this process stalls.
type election struct {
	client etcdclient.Client
	// prefix also holds the keys of instances running the previous election code,
	// so a term only starts once they have all left.
	prefix  string
	term    time.Duration
	leaseID clientv3.LeaseID
	rev     int64
	endTerm context.CancelFunc
}

func (e *election) Campaign(ctx context.Context, host string) (context.Context, error) {
	for {
		if err := e.waitForNoLeader(ctx); err != nil {
			return nil, err
		}

		start := time.Now()
		// etcd raises TTLs below its minimum, so the granted TTL is used for the deadline.
		lease, err := e.client.Grant(ctx, int64(math.Ceil(e.term.Seconds())))
		if err != nil {
			return nil, fmt.Errorf("grant lease: %w", err)
		}
		// If the put applied despite an error, the key expires with the lease.
		resp, err := e.client.Txn(ctx).
			If(clientv3.Compare(clientv3.CreateRevision(e.prefix), "=", 0).WithPrefix()).
			Then(clientv3.OpPut(e.key(lease.ID), host, clientv3.WithLease(lease.ID))).
			Commit()
		if err != nil {
			return nil, fmt.Errorf("acquire leadership: %w", err)
		}
		if !resp.Succeeded {
			// Another instance won, the unused lease expires on its own.
			continue
		}

		e.leaseID, e.rev = lease.ID, resp.Header.Revision
		// Measured from before the grant so the term ends here before etcd expires the lease.
		termCtx, endTerm := context.WithDeadline(ctx, start.Add(time.Duration(lease.TTL)*time.Second))
		e.endTerm = endTerm
		return termCtx, nil
	}
}

// waitForNoLeader returns once no key exists under the prefix.
func (e *election) waitForNoLeader(ctx context.Context) error {
	for {
		resp, err := e.client.Get(ctx, e.prefix, clientv3.WithPrefix(), clientv3.WithCountOnly())
		if err != nil {
			return fmt.Errorf("get leader: %w", err)
		}
		if resp.Count == 0 {
			return nil
		}

		// The watch starts right after the revision we read, so etcd replays a delete that
		// lands before the watch is created. Watching from now would miss it and wait for the
		// next unrelated change. If that revision is already compacted the watch errors instead.
		watchCtx, cancel := context.WithCancel(ctx)
		err = waitForDelete(watchCtx, e.client.Watch(watchCtx, e.prefix, clientv3.WithPrefix(), clientv3.WithRev(resp.Header.Revision+1)))
		cancel()
		if err != nil {
			return err
		}
	}
}

func waitForDelete(ctx context.Context, watchCh clientv3.WatchChan) error {
	for resp := range watchCh {
		if err := resp.Err(); err != nil {
			return fmt.Errorf("watch leader: %w", err)
		}
		for _, ev := range resp.Events {
			if ev.Type == mvccpb.DELETE {
				return nil
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New("leader watch closed")
}

func (e *election) Resign(ctx context.Context) error {
	if e.leaseID == clientv3.NoLease {
		return nil
	}
	e.endTerm()
	// Revoking deletes the leader key. A lease that already expired took the key with it.
	if _, err := e.client.Revoke(ctx, e.leaseID); err != nil && !errors.Is(err, rpctypes.ErrLeaseNotFound) {
		return fmt.Errorf("revoke lease: %w", err)
	}
	return nil
}

func (e *election) Guard() store.GuardFunc {
	return func(txn store.Txn) (store.Txn, error) {
		// The guard receives the generic Txn and asserts it to the concrete type it expects.
		etcdTxn, ok := txn.(clientv3.Txn)
		if !ok {
			return nil, fmt.Errorf("invalid transaction type for etcd guard: expected clientv3.Txn, got %T", txn)
		}
		// Every term writes a new key, so this fails once our term has ended.
		return etcdTxn.If(clientv3.Compare(clientv3.CreateRevision(e.key(e.leaseID)), "=", e.rev)), nil
	}
}

// key matches the layout of concurrency.Election, one key per lease under the prefix.
func (e *election) key(leaseID clientv3.LeaseID) string {
	return fmt.Sprintf("%s%x", e.prefix, leaseID)
}
