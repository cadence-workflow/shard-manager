package executorstore

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination=executorstore_mock.go ExecutorStore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/fx"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdclient"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdkeys"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/etcdtypes"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/executorstore/common"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store/etcd/executorstore/shardcache"
)

const (
	// guardOpOverhead is the number of transaction slots consumed by the leadership guard's If condition.
	guardOpOverhead = 1
)

type executorStoreImpl struct {
	client        etcdclient.Client
	prefix        string
	logger        log.Logger
	shardCache    *shardcache.ShardToExecutorCache
	timeSource    clock.TimeSource
	recordWriter  *common.RecordWriter
	cfg           *config.Config
	metricsClient metrics.Client
}

// ExecutorStoreParams defines the dependencies for the etcd store, for use with fx.
type ExecutorStoreParams struct {
	fx.In

	Client        etcdclient.Client `name:"executorstore"`
	ETCDConfig    etcdclient.ExecutorStoreConfig
	Lifecycle     fx.Lifecycle
	Logger        log.Logger
	TimeSource    clock.TimeSource
	Config        *config.Config
	MetricsClient metrics.Client
}

// NewStore creates a new etcd-backed store and provides it to the fx application.
func NewStore(p ExecutorStoreParams) (store.Store, error) {
	shardCache := shardcache.NewShardToExecutorCache(p.ETCDConfig.Prefix, p.Client, p.Logger, p.TimeSource, p.MetricsClient)

	timeSource := p.TimeSource
	if timeSource == nil {
		timeSource = clock.NewRealTimeSource()
	}

	recordWriter, err := common.NewRecordWriter(p.ETCDConfig.Compression)
	if err != nil {
		return nil, fmt.Errorf("create record writer: %w", err)
	}

	store := &executorStoreImpl{
		client:        p.Client,
		prefix:        p.ETCDConfig.Prefix,
		logger:        p.Logger,
		shardCache:    shardCache,
		timeSource:    timeSource,
		recordWriter:  recordWriter,
		cfg:           p.Config,
		metricsClient: p.MetricsClient,
	}

	p.Lifecycle.Append(fx.StartStopHook(store.Start, store.Stop))

	return store, nil
}

func (s *executorStoreImpl) Start() {
	s.shardCache.Start()
}

func (s *executorStoreImpl) Stop() {
	s.shardCache.Stop()
}

// --- HeartbeatStore Implementation ---

func (s *executorStoreImpl) RecordHeartbeat(ctx context.Context, namespace, executorID string, request store.HeartbeatState) error {
	heartbeatKey := etcdkeys.BuildExecutorKey(s.prefix, namespace, executorID, etcdkeys.ExecutorHeartbeatKey)
	stateKey := etcdkeys.BuildExecutorKey(s.prefix, namespace, executorID, etcdkeys.ExecutorStatusKey)
	reportedShardsKey := etcdkeys.BuildExecutorKey(s.prefix, namespace, executorID, etcdkeys.ExecutorReportedShardsKey)

	reportedShardsData, err := json.Marshal(request.ReportedShards)
	if err != nil {
		return fmt.Errorf("marshal reported shards: %w", err)
	}

	jsonState, err := json.Marshal(request.Status)
	if err != nil {
		return fmt.Errorf("marshal assinged state: %w", err)
	}

	// Compress data before writing to etcd
	compressedReportedShards, err := s.recordWriter.Write(reportedShardsData)
	if err != nil {
		return fmt.Errorf("compress reported shards: %w", err)
	}

	compressedState, err := s.recordWriter.Write(jsonState)
	if err != nil {
		return fmt.Errorf("compress assigned state: %w", err)
	}

	// Build all operations including metadata
	ops := []clientv3.Op{
		clientv3.OpPut(heartbeatKey, etcdtypes.FormatTime(request.LastHeartbeat)),
		clientv3.OpPut(stateKey, string(compressedState)),
		clientv3.OpPut(reportedShardsKey, string(compressedReportedShards)),
	}
	for key, value := range request.Metadata {
		metadataKey := etcdkeys.BuildMetadataKey(s.prefix, namespace, executorID, key)
		ops = append(ops, clientv3.OpPut(metadataKey, value))
	}

	// Atomically update both the timestamp and the state.
	_, err = s.client.Txn(ctx).Then(ops...).Commit()

	if err != nil {
		return fmt.Errorf("record heartbeat: %w", err)
	}
	return nil
}

// RecordShardStatistics persists prepared statistics if the executor's
// assignment has not changed since it was read.
func (s *executorStoreImpl) RecordShardStatistics(
	ctx context.Context,
	namespace string,
	executorID string,
	assignmentModRevision int64,
	statistics map[string]store.ShardStatistics,
) error {
	storedStatistics := etcdtypes.FromShardStatisticsMap(statistics)

	payload, err := json.Marshal(storedStatistics)
	if err != nil {
		return fmt.Errorf("marshal executor shard statistics: %w", err)
	}
	compressedPayload, err := s.recordWriter.Write(payload)
	if err != nil {
		return fmt.Errorf("compress executor shard statistics: %w", err)
	}

	assignedStateKey := etcdkeys.BuildExecutorKey(s.prefix, namespace, executorID, etcdkeys.ExecutorAssignedStateKey)
	statsKey := etcdkeys.BuildExecutorKey(s.prefix, namespace, executorID, etcdkeys.ExecutorShardStatisticsKey)
	txnResp, err := s.client.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(assignedStateKey), "=", assignmentModRevision)).
		Then(clientv3.OpPut(statsKey, string(compressedPayload))).
		Commit()
	if err != nil {
		return fmt.Errorf("put executor shard statistics: %w", err)
	}
	if !txnResp.Succeeded {
		return fmt.Errorf("%w: executor assignment changed", store.ErrVersionConflict)
	}
	return nil
}

// GetExecutorState retrieves the persisted state for a single executor.
func (s *executorStoreImpl) GetExecutorState(ctx context.Context, namespace string, executorID string) (store.ExecutorState, error) {
	// The prefix for all keys related to a single executor.
	executorIDPrefix := etcdkeys.BuildExecutorIDPrefix(s.prefix, namespace, executorID)
	resp, err := s.client.Get(ctx, executorIDPrefix, clientv3.WithPrefix())
	if err != nil {
		return store.ExecutorState{}, fmt.Errorf("etcd get failed for executor %s: %w", executorID, err)
	}

	if resp.Count == 0 {
		return store.ExecutorState{}, store.ErrExecutorNotFound
	}

	parsedData, err := common.ParseExecutorKVs(s.prefix, namespace, resp.Kvs)
	if err != nil {
		return store.ExecutorState{}, err
	}

	executorData, ok := parsedData[executorID]
	if !ok {
		return store.ExecutorState{}, store.ErrExecutorNotFound
	}

	heartbeatState := &store.HeartbeatState{
		LastHeartbeat:  executorData.LastHeartbeat.ToTime(),
		Status:         executorData.Status,
		ReportedShards: executorData.ReportedShards,
		Metadata:       executorData.Metadata,
	}

	var assignedState *store.AssignedState
	if executorData.AssignedState != nil {
		assignedState = executorData.AssignedState.ToAssignedState()
	}

	statistics := etcdtypes.ToShardStatisticsMap(executorData.Statistics)

	return store.ExecutorState{
		Heartbeat:  heartbeatState,
		Assignment: assignedState,
		Statistics: statistics,
	}, nil
}

// --- ShardStore Implementation ---

func (s *executorStoreImpl) GetState(ctx context.Context, namespace string) (*store.NamespaceState, error) {
	heartbeatStates := make(map[string]store.HeartbeatState)
	assignedStates := make(map[string]store.AssignedState)
	shardStats := make(map[string]store.ShardStatistics)

	metricsScope := s.metricsClient.Scope(
		metrics.ShardDistributorStoreGetStateScope,
		metrics.NamespaceTag(namespace),
	)
	txn := s.client.Txn(ctx).Then(
		clientv3.OpGet(etcdkeys.BuildExecutorsPrefix(s.prefix, namespace), clientv3.WithPrefix()),
		clientv3.OpGet(etcdkeys.BuildDrainedShardsPrefix(s.prefix, namespace), clientv3.WithPrefix()),
		clientv3.OpGet(etcdkeys.BuildDrainedHostsPrefix(s.prefix, namespace), clientv3.WithPrefix()),
	)
	start := s.timeSource.Now()
	txnResp, err := txn.Commit()
	metricsScope.RecordHistogramDuration(metrics.ShardDistributorStoreGetStateETCDRoundTripLatency, s.timeSource.Since(start))
	if err != nil {
		return nil, fmt.Errorf("get namespace state: %w", err)
	}
	if len(txnResp.Responses) != 3 {
		return nil, fmt.Errorf("get namespace state: expected 3 responses, got %d", len(txnResp.Responses))
	}

	parsedData, err := common.ParseExecutorKVs(s.prefix, namespace, txnResp.Responses[0].GetResponseRange().Kvs)
	if err != nil {
		return nil, err
	}

	for executorID, executorData := range parsedData {
		heartbeatStates[executorID] = store.HeartbeatState{
			LastHeartbeat:  executorData.LastHeartbeat.ToTime(),
			Status:         executorData.Status,
			ReportedShards: executorData.ReportedShards,
			Metadata:       executorData.Metadata,
		}

		if executorData.AssignedState != nil {
			assignedStates[executorID] = *executorData.AssignedState.ToAssignedState()
		}

		for shardID, statistics := range executorData.Statistics {
			shardStats[shardID] = *statistics.ToShardStatistics()
		}
	}

	return &store.NamespaceState{
		Executors:        heartbeatStates,
		ShardStats:       shardStats,
		ShardAssignments: assignedStates,
		DrainedShards:    s.parseDrainedShardKVs(namespace, txnResp.Responses[1].GetResponseRange().Kvs),
		DrainedHosts:     s.parseDrainedHostKVs(namespace, txnResp.Responses[2].GetResponseRange().Kvs),
	}, nil
}

// loadDrainedShardSet reads every drained-shard key for the namespace and returns
// them as a set.
func (s *executorStoreImpl) loadDrainedShardSet(ctx context.Context, namespace string) (map[string]struct{}, error) {
	drainedPrefix := etcdkeys.BuildDrainedShardsPrefix(s.prefix, namespace)
	resp, err := s.client.Get(ctx, drainedPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("get drained shards prefix: %w", err)
	}
	return s.parseDrainedShardKVs(namespace, resp.Kvs), nil
}

// parseDrainedShardKVs turns drained-shard keys into a set of shard IDs.
// Malformed keys are skipped to not stall the rebalance loop.
func (s *executorStoreImpl) parseDrainedShardKVs(namespace string, kvs []*mvccpb.KeyValue) map[string]struct{} {
	drained := make(map[string]struct{}, len(kvs))
	for _, kv := range kvs {
		shardID, err := etcdkeys.ParseDrainedShardKey(s.prefix, namespace, string(kv.Key))
		if err != nil {
			s.logger.Warn("skipping malformed drained shard key",
				tag.ShardNamespace(namespace),
				tag.Key(string(kv.Key)),
				tag.Error(err),
			)
			continue
		}
		drained[shardID] = struct{}{}
	}
	return drained
}

// loadDrainedHostSet reads every drained-host key for the namespace.
func (s *executorStoreImpl) loadDrainedHostSet(ctx context.Context, namespace string) (map[string]store.DrainedHost, error) {
	drainedPrefix := etcdkeys.BuildDrainedHostsPrefix(s.prefix, namespace)
	resp, err := s.client.Get(ctx, drainedPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("get drained hosts prefix: %w", err)
	}
	return s.parseDrainedHostKVs(namespace, resp.Kvs), nil
}

// parseDrainedHostKVs turns drained-host keys into hostname -> metadata
func (s *executorStoreImpl) parseDrainedHostKVs(namespace string, kvs []*mvccpb.KeyValue) map[string]store.DrainedHost {
	drained := make(map[string]store.DrainedHost, len(kvs))
	for _, kv := range kvs {
		hostname, err := etcdkeys.ParseDrainedHostKey(s.prefix, namespace, string(kv.Key))
		if err != nil {
			s.logger.Warn("skipping malformed drained host key",
				tag.ShardNamespace(namespace),
				tag.Key(string(kv.Key)),
				tag.Error(err),
			)
			continue
		}
		record := store.DrainedHost{Hostname: hostname}
		if len(kv.Value) > 0 {
			var parsed store.DrainedHost
			if err := json.Unmarshal(kv.Value, &parsed); err != nil {
				s.logger.Warn("skipping malformed drained host value",
					tag.ShardNamespace(namespace),
					tag.Key(string(kv.Key)),
					tag.Error(err),
				)
			} else {
				parsed.Hostname = hostname
				record = parsed
			}
		}
		drained[hostname] = record
	}
	return drained
}

func (s *executorStoreImpl) SubscribeToAssignmentChanges(ctx context.Context, namespace string) (<-chan struct{}, func(), error) {
	return s.shardCache.Subscribe(namespace)
}

func (s *executorStoreImpl) GetShardAssignments(namespace string) (store.AssignmentSnapshot, error) {
	return s.shardCache.GetShardAssignments(namespace)
}

func (s *executorStoreImpl) SubscribeToExecutorStatusChanges(ctx context.Context, namespace string) (<-chan int64, error) {
	revisionChan := make(chan int64, 1)

	go func() {
		defer close(revisionChan)

		scope := s.metricsClient.Scope(metrics.ShardDistributorWatchScope).
			Tagged(metrics.NamespaceTag(namespace)).
			Tagged(metrics.ShardDistributorWatchTypeTag("rebalance"))

		watchChan := s.client.Watch(ctx,
			etcdkeys.BuildExecutorsPrefix(s.prefix, namespace),
			clientv3.WithPrefix(),
			clientv3.WithPrevKV(),
		)

		for watchResp := range watchChan {
			if err := watchResp.Err(); err != nil {
				return
			}

			// Track watch metrics
			sw := scope.StartTimer(metrics.ShardDistributorWatchProcessingLatency)
			scope.AddCounter(metrics.ShardDistributorWatchEventsReceived, int64(len(watchResp.Events)))

			if !s.hasExecutorStatusChanged(watchResp, namespace) {
				sw.Stop()
				continue
			}

			// If the channel is full, it means the previous revision hasn't been processed yet.
			// Pop the old revision to make room for the new one, ensuring we always have the latest revision.
			select {
			case <-revisionChan:
			default:
			}

			revisionChan <- watchResp.Header.Revision
			sw.Stop()
		}
	}()

	return revisionChan, nil
}

// hasExecutorStatusChanged checks if any of the events in the watch response correspond to changes in executor status.
func (s *executorStoreImpl) hasExecutorStatusChanged(watchResp clientv3.WatchResponse, namespace string) bool {
	for _, event := range watchResp.Events {
		_, keyType, err := etcdkeys.ParseExecutorKey(s.prefix, namespace, string(event.Kv.Key))
		if err != nil {
			s.logger.Warn("Received watch event with unrecognized key format", tag.Key(string(event.Kv.Key)))
			continue
		}

		// Only consider changes to the ExecutorStatusKey as significant for triggering a revision update.
		if keyType != etcdkeys.ExecutorStatusKey {
			continue
		}

		// If the previous value is the same as the new value, it means the status didn't actually change
		if event.PrevKv != nil && string(event.PrevKv.Value) == string(event.Kv.Value) {
			continue
		}

		return true
	}

	return false
}

func (s *executorStoreImpl) AssignShards(ctx context.Context, namespace string, request store.AssignShardsRequest, guard store.GuardFunc) error {
	var ops []clientv3.Op
	var opsElse []clientv3.Op
	var comparisons []clientv3.Cmp
	comparisonMaps := make(map[string]int64)

	// 1. Prepare operations to delete stale executors and add comparisons to ensure they haven't been modified
	for executorID, expectedModRevision := range request.ExecutorsToDelete {
		// Build the assigned state key to check for concurrent modifications
		executorStateKey := etcdkeys.BuildExecutorKey(s.prefix, namespace, executorID, etcdkeys.ExecutorAssignedStateKey)

		// Add a comparison to ensure the executor's assigned state hasn't changed
		// This prevents deleting an executor that just received a shard assignment
		comparisons = append(comparisons, clientv3.Compare(clientv3.ModRevision(executorStateKey), "=", expectedModRevision))
		comparisonMaps[executorStateKey] = expectedModRevision

		// Delete all keys for this executor
		executorPrefix := etcdkeys.BuildExecutorIDPrefix(s.prefix, namespace, executorID)
		ops = append(ops, clientv3.OpDelete(executorPrefix, clientv3.WithPrefix()))
		opsElse = append(opsElse, clientv3.OpGet(executorStateKey))
	}

	// 2. Prepare operations to update executor states and shard ownership,
	// and comparisons to check for concurrent modifications.
	// All executors get a ModRevision comparison (optimistic lock), but only
	// changed executors get an OpPut. When ChangedExecutors is nil, all
	// executors are written (backwards-compatible default).
	for executorID, state := range request.NewState.ShardAssignments {
		executorStateKey := etcdkeys.BuildExecutorKey(s.prefix, namespace, executorID, etcdkeys.ExecutorAssignedStateKey)

		comparisons = append(comparisons, clientv3.Compare(clientv3.ModRevision(executorStateKey), "=", state.ModRevision))
		comparisonMaps[executorStateKey] = state.ModRevision
		opsElse = append(opsElse, clientv3.OpGet(executorStateKey))

		_, isChanged := request.ChangedExecutors[executorID]
		if request.ChangedExecutors != nil && !isChanged {
			continue
		}

		value, err := json.Marshal(etcdtypes.FromAssignedState(&state))
		if err != nil {
			return fmt.Errorf("marshal assigned shards for executor %s: %w", executorID, err)
		}

		compressedValue, err := s.recordWriter.Write(value)
		if err != nil {
			return fmt.Errorf("compress assigned shards for executor %s: %w", executorID, err)
		}
		ops = append(ops, clientv3.OpPut(executorStateKey, string(compressedValue)))
	}

	if len(ops) == 0 {
		return nil
	}

	// 3. Apply the guard function to get the base transaction, which may already have an 'If' condition for leadership.
	nativeTxn := s.client.Txn(ctx)
	guardedTxn, err := guard(nativeTxn)
	if err != nil {
		return fmt.Errorf("apply transaction guard: %w", err)
	}
	etcdGuardedTxn, ok := guardedTxn.(clientv3.Txn)
	if !ok {
		return fmt.Errorf("guard function returned invalid transaction type")
	}

	// 4. Create a nested transaction operation. This allows us to add our own 'If' (comparisons)
	// and 'Then' (ops) logic that will only execute if the outer guard's 'If' condition passes.
	// we catch what is the state in the else operations so we can identify which part of the condition failed
	nestedTxnOp := clientv3.OpTxn(
		comparisons, // Our IF conditions
		ops,         // Our THEN operations
		opsElse,     // Our ELSE operations
	)

	// 5. Add the nested transaction to the guarded transaction's THEN clause and commit.
	etcdGuardedTxn = etcdGuardedTxn.Then(nestedTxnOp)
	txnResp, err := etcdGuardedTxn.Commit()
	if err != nil {
		return fmt.Errorf("commit shard assignments transaction: %w", err)
	}

	// 6. Check the results of both the outer and nested transactions.
	if !txnResp.Succeeded {
		// This means the guard's condition (e.g., leadership) failed.
		return fmt.Errorf("%w: transaction failed, leadership may have changed", store.ErrVersionConflict)
	}

	// The guard's condition passed. Now check if our nested transaction succeeded.
	// Since we only have one Op in our 'Then', we check the first response.
	if len(txnResp.Responses) == 0 {
		return fmt.Errorf("unexpected empty response from transaction")
	}

	nestedResp := txnResp.Responses[0].GetResponseTxn()
	if !nestedResp.Succeeded {
		// This means our revision checks failed.
		failingRevisionString := ""
		for _, keyValue := range nestedResp.Responses[0].GetResponseRange().Kvs {
			expectedValue, ok := comparisonMaps[string(keyValue.Key)]
			if !ok || expectedValue != keyValue.ModRevision {
				failingRevisionString = failingRevisionString + fmt.Sprintf("{ key: %s, expected:%v, actual: %v }", string(keyValue.Key), expectedValue, keyValue.ModRevision)
			}
		}
		return fmt.Errorf("%w: transaction failed, a shard may have been concurrently assigned, %v", store.ErrVersionConflict, failingRevisionString)
	}

	return nil
}

// commitOps commits the given operations in batches to stay within etcd's
// per-transaction operation limit, returning each batch's response in submission
// order so callers can inspect per-op outcomes.
//
// Each batch is a separate guarded transaction, so a failure part-way through returns
// immediately and leaves earlier batches committed.
func (s *executorStoreImpl) commitOps(ctx context.Context, ops []clientv3.Op, guard store.GuardFunc) ([]*clientv3.TxnResponse, error) {
	if len(ops) == 0 {
		return nil, nil
	}
	maxOpsPerTxn := s.cfg.MaxEtcdTxnOps() - guardOpOverhead
	if maxOpsPerTxn < 1 {
		maxOpsPerTxn = 1
	}

	numBatches := (len(ops) + maxOpsPerTxn - 1) / maxOpsPerTxn
	batchSize := (len(ops) + numBatches - 1) / numBatches

	responses := make([]*clientv3.TxnResponse, 0, numBatches)
	for i := 0; i < len(ops); i += batchSize {
		end := min(i+batchSize, len(ops))

		guardedTxn, err := guard(s.client.Txn(ctx))
		if err != nil {
			return nil, fmt.Errorf("apply transaction guard: %w", err)
		}
		etcdGuardedTxn, ok := guardedTxn.(clientv3.Txn)
		if !ok {
			return nil, fmt.Errorf("guard function returned invalid transaction type")
		}

		resp, err := etcdGuardedTxn.Then(ops[i:end]...).Commit()
		if err != nil {
			return nil, fmt.Errorf("commit batch: %w", err)
		}
		if !resp.Succeeded {
			return nil, fmt.Errorf("transaction failed, leadership may have changed")
		}
		responses = append(responses, resp)
	}
	return responses, nil
}

// deletedIDs returns the IDs whose delete ops actually removed a key.
// responses are in the same order as ids; commitOps preserves that across batches.
func deletedIDs(responses []*clientv3.TxnResponse, ids []string) ([]string, error) {
	removed := make([]string, 0, len(ids))
	opIdx := 0
	for _, resp := range responses {
		for _, opResp := range resp.Responses {
			if opIdx >= len(ids) {
				return nil, fmt.Errorf("got more op responses than the %d ops submitted", len(ids))
			}
			if del := opResp.GetResponseDeleteRange(); del != nil && del.Deleted > 0 {
				removed = append(removed, ids[opIdx])
			}
			opIdx++
		}
	}
	return removed, nil
}

// DeleteExecutors deletes the given executors from the store. It does not delete the shards owned by the executors, this
// should be handled by the namespace processor loop as we want to reassign, not delete the shards.
func (s *executorStoreImpl) DeleteExecutors(ctx context.Context, namespace string, executorIDs []string, guard store.GuardFunc) error {
	if len(executorIDs) == 0 {
		return nil
	}
	ops := make([]clientv3.Op, 0, len(executorIDs))

	for _, executorID := range executorIDs {
		executorIDPrefix := etcdkeys.BuildExecutorIDPrefix(s.prefix, namespace, executorID)
		ops = append(ops, clientv3.OpDelete(executorIDPrefix, clientv3.WithPrefix()))
	}

	if _, err := s.commitOps(ctx, ops, guard); err != nil {
		return fmt.Errorf("delete executors: %w", err)
	}
	return nil
}

func (s *executorStoreImpl) DeleteAssignedStates(ctx context.Context, namespace string, executorIDs []string, guard store.GuardFunc) error {
	if len(executorIDs) == 0 {
		return nil
	}
	ops := make([]clientv3.Op, 0, len(executorIDs))

	for _, executorID := range executorIDs {
		executorIDPrefix := etcdkeys.BuildExecutorKey(s.prefix, namespace, executorID, etcdkeys.ExecutorAssignedStateKey)
		ops = append(ops, clientv3.OpDelete(executorIDPrefix, clientv3.WithPrefix()))
	}

	if _, err := s.commitOps(ctx, ops, guard); err != nil {
		return fmt.Errorf("delete assigned states: %w", err)
	}
	return nil
}

// DeleteShardStats deletes shard statistics for the given shard IDs.
// If the operation fails (e.g. due to leadership loss), it returns immediately.
// Partial deletions are acceptable as the periodic cleanup loop will retry remaining keys.
func (s *executorStoreImpl) DeleteShardStats(ctx context.Context, namespace string, shardIDs []string, guard store.GuardFunc) error {
	if len(shardIDs) == 0 {
		return nil
	}

	// Build a lookup for shard IDs to delete.
	toDelete := make(map[string]struct{}, len(shardIDs))
	for _, shardID := range shardIDs {
		toDelete[shardID] = struct{}{}
	}

	// Fetch all statistics at the executor level for this namespace.
	executorPrefix := etcdkeys.BuildExecutorsPrefix(s.prefix, namespace)
	resp, err := s.client.Get(ctx, executorPrefix, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("get executor data for shard stats deletion: %w", err)
	}

	ops := make([]clientv3.Op, 0)

	parsedData, err := common.ParseExecutorKVs(s.prefix, namespace, resp.Kvs)
	if err != nil {
		return err
	}

	for executorID, executorData := range parsedData {
		executorStats := executorData.Statistics

		changed := false
		for shardID := range executorStats {
			if _, ok := toDelete[shardID]; ok {
				delete(executorStats, shardID)
				changed = true
			}
		}

		if !changed {
			continue
		}

		statsKey := etcdkeys.BuildExecutorKey(s.prefix, namespace, executorID, etcdkeys.ExecutorShardStatisticsKey)
		if len(executorStats) == 0 {
			ops = append(ops, clientv3.OpDelete(statsKey))
			continue
		}

		payload, err := json.Marshal(executorStats)
		if err != nil {
			s.logger.Warn(
				"failed to marshal executor shard statistics during cleanup",
				tag.ShardNamespace(namespace),
				tag.ShardExecutor(executorID),
				tag.Error(err),
			)
			continue
		}

		compressedPayload, err := s.recordWriter.Write(payload)
		if err != nil {
			s.logger.Warn(
				"failed to compress executor shard statistics during cleanup",
				tag.ShardNamespace(namespace),
				tag.ShardExecutor(executorID),
				tag.Error(err),
			)
			continue
		}

		ops = append(ops, clientv3.OpPut(statsKey, string(compressedPayload)))
	}

	if _, err := s.commitOps(ctx, ops, guard); err != nil {
		return fmt.Errorf("delete shard stats: %w", err)
	}
	return nil
}

// GetShardOwner returns the owner of the shard.
// Drained shards return ErrShardDrained rather than the last assigned owner
func (s *executorStoreImpl) GetShardOwner(ctx context.Context, namespace, shardID string) (*store.ShardOwner, error) {
	drained, err := s.shardCache.IsShardDrained(ctx, namespace, shardID)
	if err != nil {
		return nil, fmt.Errorf("check shard drained: %w", err)
	}
	if drained {
		return nil, store.ErrShardDrained
	}

	return s.shardCache.GetShardOwner(ctx, namespace, shardID)
}

// ResetNamespace deletes every key under <prefix>/<namespace>/ in a single
// etcd op. This wipes the leader key, executor heartbeats/status/metadata,
// shard assignments, shard statistics, drained shards, and drained hosts.
// It is intentionally NOT guarded by leadership: any concurrent leader write
// will subsequently fail its own leadership-key revision check, which is the
// desired behaviour.
func (s *executorStoreImpl) ResetNamespace(ctx context.Context, namespace string) (int64, error) {
	prefix := etcdkeys.BuildNamespacePrefix(s.prefix, namespace)
	resp, err := s.client.Delete(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return 0, fmt.Errorf("delete namespace prefix %q: %w", prefix, err)
	}
	return resp.Deleted, nil
}

func (s *executorStoreImpl) GetExecutor(ctx context.Context, namespace string, executorID string) (*store.ShardOwner, error) {
	return s.shardCache.GetExecutor(ctx, namespace, executorID)
}

// DrainShards writes one empty-valued key per shard under the namespace's drained
// prefix.
// Draining is deliberately not guarded by leadership: it is an operator
// action that must work regardless of which host is currently the leader, and the
// leader picks the change up on its next rebalance.
func (s *executorStoreImpl) DrainShards(ctx context.Context, namespace string, shardIDs []string) error {
	if len(shardIDs) == 0 {
		return nil
	}

	ops := make([]clientv3.Op, 0, len(shardIDs))
	for _, shardID := range shardIDs {
		if err := etcdkeys.ValidateShardID(shardID); err != nil {
			return fmt.Errorf("drain shards: %w", err)
		}
		ops = append(ops, clientv3.OpPut(etcdkeys.BuildDrainedShardKey(s.prefix, namespace, shardID), ""))
	}

	if _, err := s.commitOps(ctx, ops, store.NopGuard()); err != nil {
		return fmt.Errorf("drain shards: %w", err)
	}
	return nil
}

// UndrainShards deletes the given drained-shard keys and reports which ones it
// actually removed.
//
// Every input key is deleted unconditionally, and the per-op DeleteRange count is
// inspected afterward: a count of 1 means this call performed the removal, 0 means
// the key was already gone (never drained, or already removed)
func (s *executorStoreImpl) UndrainShards(ctx context.Context, namespace string, shardIDs []string) ([]string, error) {
	if len(shardIDs) == 0 {
		return nil, nil
	}

	ops := make([]clientv3.Op, 0, len(shardIDs))
	for _, shardID := range shardIDs {
		if err := etcdkeys.ValidateShardID(shardID); err != nil {
			return nil, fmt.Errorf("undrain shards: %w", err)
		}
		ops = append(ops, clientv3.OpDelete(etcdkeys.BuildDrainedShardKey(s.prefix, namespace, shardID)))
	}

	responses, err := s.commitOps(ctx, ops, store.NopGuard())
	if err != nil {
		return nil, fmt.Errorf("undrain shards: %w", err)
	}

	removed, err := deletedIDs(responses, shardIDs)
	if err != nil {
		return nil, fmt.Errorf("undrain shards: %w", err)
	}
	return removed, nil
}

// GetDrainedShards returns the shards currently drained for the namespace.
func (s *executorStoreImpl) GetDrainedShards(ctx context.Context, namespace string) ([]string, error) {
	drained, err := s.loadDrainedShardSet(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("get drained shards: %w", err)
	}
	return slices.Sorted(maps.Keys(drained)), nil
}

// DrainHosts writes one JSON-valued key per host under the namespace's drained
// hosts prefix
func (s *executorStoreImpl) DrainHosts(ctx context.Context, namespace string, hosts []store.DrainedHost) error {
	if len(hosts) == 0 {
		return nil
	}

	validated, err := validateDrainedHosts(hosts, s.timeSource.Now().UTC())
	if err != nil {
		return fmt.Errorf("drain hosts: %w", err)
	}

	existing, err := s.loadDrainedHostSet(ctx, namespace)
	if err != nil {
		return fmt.Errorf("drain hosts: %w", err)
	}

	ops := make([]clientv3.Op, 0, len(validated))
	for hostname, host := range validated {
		if _, already := existing[hostname]; already {
			continue
		}
		value, err := json.Marshal(host)
		if err != nil {
			return fmt.Errorf("drain hosts: marshal %s: %w", hostname, err)
		}
		ops = append(ops, clientv3.OpPut(etcdkeys.BuildDrainedHostKey(s.prefix, namespace, hostname), string(value)))
	}
	if len(ops) == 0 {
		return nil
	}

	if _, err := s.commitOps(ctx, ops, store.NopGuard()); err != nil {
		return fmt.Errorf("drain hosts: %w", err)
	}
	return nil
}

// UndrainHosts deletes the given drained-host keys and reports which ones it
// actually removed.
func (s *executorStoreImpl) UndrainHosts(ctx context.Context, namespace string, hostnames []string) ([]string, error) {
	if len(hostnames) == 0 {
		return nil, nil
	}

	for _, hostname := range hostnames {
		if err := etcdkeys.ValidateHostname(hostname); err != nil {
			return nil, fmt.Errorf("undrain hosts: %w", err)
		}
	}

	ops := make([]clientv3.Op, 0, len(hostnames))
	for _, hostname := range hostnames {
		ops = append(ops, clientv3.OpDelete(etcdkeys.BuildDrainedHostKey(s.prefix, namespace, hostname)))
	}

	responses, err := s.commitOps(ctx, ops, store.NopGuard())
	if err != nil {
		return nil, fmt.Errorf("undrain hosts: %w", err)
	}

	removed, err := deletedIDs(responses, hostnames)
	if err != nil {
		return nil, fmt.Errorf("undrain hosts: %w", err)
	}
	return removed, nil
}

// GetDrainedHosts returns the hosts currently drained for the namespace, sorted by hostname
func (s *executorStoreImpl) GetDrainedHosts(ctx context.Context, namespace string) ([]store.DrainedHost, error) {
	drained, err := s.loadDrainedHostSet(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("get drained hosts: %w", err)
	}
	hosts := make([]store.DrainedHost, 0, len(drained))
	for _, hostname := range slices.Sorted(maps.Keys(drained)) {
		hosts = append(hosts, drained[hostname])
	}
	return hosts, nil
}

// validateDrainedHosts rejects unusable hostnames and collapses duplicates,
// keeping the first occurrence of each host.
func validateDrainedHosts(hosts []store.DrainedHost, now time.Time) (map[string]store.DrainedHost, error) {
	validated := make(map[string]store.DrainedHost, len(hosts))
	for _, host := range hosts {
		if err := etcdkeys.ValidateHostname(host.Hostname); err != nil {
			return nil, err
		}
		if _, exists := validated[host.Hostname]; exists {
			continue
		}
		if host.DrainedAt.IsZero() {
			host.DrainedAt = now
		}
		validated[host.Hostname] = host
	}
	return validated, nil
}

// RecordShardStatisticsBatch records complete statistics maps for multiple
// executors.
func (s *executorStoreImpl) RecordShardStatisticsBatch(ctx context.Context, namespace string, updates []store.ExecutorShardStatistics) error {
	var multiError error
	for _, update := range updates {
		statsKey := etcdkeys.BuildExecutorKey(s.prefix, namespace, update.ExecutorID, etcdkeys.ExecutorShardStatisticsKey)

		if len(update.Statistics) == 0 {
			if _, err := s.client.Delete(ctx, statsKey); err != nil {
				multiError = errors.Join(multiError, fmt.Errorf("failed to delete shard statistics for executor %s: %w", update.ExecutorID, err))
			}
			continue
		}

		storedStatistics := etcdtypes.FromShardStatisticsMap(update.Statistics)
		payload, err := json.Marshal(storedStatistics)
		if err != nil {
			multiError = errors.Join(multiError, fmt.Errorf("failed to marshal shard statistics for executor %s: %w", update.ExecutorID, err))
			continue
		}

		compressedPayload, err := s.recordWriter.Write(payload)
		if err != nil {
			multiError = errors.Join(multiError, fmt.Errorf("failed to compress shard statistics for executor %s: %w", update.ExecutorID, err))
			continue
		}

		if _, err := s.client.Put(ctx, statsKey, string(compressedPayload)); err != nil {
			multiError = errors.Join(multiError, fmt.Errorf("failed to put shard statistics for executor %s: %w", update.ExecutorID, err))
		}
	}
	return multiError
}
