package executorstore

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdkeys"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdtypes"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/executorstore/common"
)

func (s *executorStoreImpl) ListRanges(ctx context.Context, namespace string) ([]*store.RangeAssignment, error) {
	resp, err := s.client.Get(ctx, etcdkeys.BuildRangesPrefix(s.prefix, namespace), clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("list ranges: %w", err)
	}

	assignments := make([]*store.RangeAssignment, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		rangeID, err := etcdkeys.ParseRangeKey(s.prefix, namespace, string(kv.Key))
		if err != nil {
			return nil, fmt.Errorf("parse range key %q: %w", string(kv.Key), err)
		}

		var stored etcdtypes.RangeAssignment
		if err := common.DecompressAndUnmarshal(kv.Value, &stored); err != nil {
			return nil, fmt.Errorf("unmarshal range assignment for key %q: %w", string(kv.Key), err)
		}

		assignment, err := stored.ToRangeAssignment(rangeID)
		if err != nil {
			return nil, fmt.Errorf("decode range assignment for key %q: %w", string(kv.Key), err)
		}
		assignment.ModRevision = kv.ModRevision
		assignments = append(assignments, assignment)
	}

	slices.SortFunc(assignments, func(a, b *store.RangeAssignment) int {
		return bytes.Compare(a.Range.LowInclusive, b.Range.LowInclusive)
	})
	return assignments, nil
}

// TODO(range-cover): write the full resulting state (compressed via s.recordWriter)
// in one transaction guarded by guard (election.Guard() in production, never NopGuard),
// after validating the whole resulting cover. Compare ModRevision for existing ranges
// and respect the txn op limit.
func (s *executorStoreImpl) AssignRanges(ctx context.Context, namespace string, request store.AssignRangesRequest, guard store.GuardFunc) error {
	return store.ErrRangeNotImplemented
}
