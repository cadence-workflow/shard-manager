package executorstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdkeys"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdtypes"
)

func (s *executorStoreImpl) ListRanges(ctx context.Context, namespace string) ([]*store.RangeAssignment, error) {
	resp, err := s.client.Get(ctx, etcdkeys.BuildRangesPrefix(s.prefix, namespace), clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("list ranges: %w", err)
	}

	assignments := make([]*store.RangeAssignment, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		assignment, err := s.parseRangeKV(namespace, kv)
		if err != nil {
			return nil, err
		}
		assignments = append(assignments, assignment)
	}

	slices.SortFunc(assignments, func(a, b *store.RangeAssignment) int {
		return bytes.Compare(a.Range.LowInclusive, b.Range.LowInclusive)
	})
	return assignments, nil
}

// parseRangeKVs skips malformed range keys. Cover validation rejects any gap a skipped key leaves.
func (s *executorStoreImpl) parseRangeKVs(namespace string, kvs []*mvccpb.KeyValue) []*store.RangeAssignment {
	assignments := make([]*store.RangeAssignment, 0, len(kvs))
	for _, kv := range kvs {
		assignment, err := s.parseRangeKV(namespace, kv)
		if err != nil {
			s.logger.Warn("skipping malformed range key",
				tag.ShardNamespace(namespace),
				tag.Key(string(kv.Key)),
				tag.Error(err),
			)
			continue
		}
		assignments = append(assignments, assignment)
	}
	return assignments
}

func (s *executorStoreImpl) parseRangeKV(namespace string, kv *mvccpb.KeyValue) (*store.RangeAssignment, error) {
	rangeID, err := etcdkeys.ParseRangeKey(s.prefix, namespace, string(kv.Key))
	if err != nil {
		return nil, fmt.Errorf("parse range key %q: %w", string(kv.Key), err)
	}

	var stored etcdtypes.RangeAssignment
	if err := json.Unmarshal(kv.Value, &stored); err != nil {
		return nil, fmt.Errorf("unmarshal range assignment for key %q: %w", string(kv.Key), err)
	}

	assignment, err := stored.ToRangeAssignment(rangeID)
	if err != nil {
		return nil, fmt.Errorf("decode range assignment for key %q: %w", string(kv.Key), err)
	}
	assignment.ModRevision = kv.ModRevision
	return assignment, nil
}

// TODO(range-cover): write the resulting cover in a single guarded
// transaction after validating the full cover. Compare ModRevision for
// existing ranges and respect the transaction op limit.
func (s *executorStoreImpl) AssignRanges(ctx context.Context, namespace string, request store.AssignRangesRequest, guard store.GuardFunc) error {
	return store.ErrRangeNotImplemented
}
