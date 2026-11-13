// Package memory implements store.Store in process memory.
package memory

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/hostname"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

var (
	errStopped     = errors.New("store is stopping")
	errGuardFailed = errors.New("transaction failed, leadership may have changed")
)

var _ store.Store = (*Store)(nil)

// Store keeps all state in process memory. Nothing survives a restart and nothing is
// shared between processes, so every component of a deployment must use the same
// instance.
//
// Revisions follow the etcd model the rest of the service is written against: one
// counter for the whole store, advanced by every committed write, with each record
// the write touched stamped with the new value.
type Store struct {
	timeSource clock.TimeSource

	mu            sync.Mutex
	revision      int64
	namespaces    map[string]*namespaceData
	namespaceSubs map[string][]chan struct{}
	statusSubs    map[string]map[chan int64]struct{}
	stopped       bool
	stopCh        chan struct{}
}

// NewStore returns an empty store. Stop closes its subscriptions.
func NewStore(timeSource clock.TimeSource) *Store {
	return &Store{
		timeSource:    timeSource,
		namespaces:    make(map[string]*namespaceData),
		namespaceSubs: make(map[string][]chan struct{}),
		statusSubs:    make(map[string]map[chan int64]struct{}),
		stopCh:        make(chan struct{}),
	}
}

type namespaceData struct {
	executors     map[string]*executorData
	drainedShards map[string]struct{}
	drainedHosts  map[string]store.DrainedHost
	// assignmentRevision is the revision of the latest write spectators can see.
	assignmentRevision int64
}

// executorData holds the records of one executor. A field is nil while its record is
// absent, and the executor exists for as long as any record does.
type executorData struct {
	heartbeat    *heartbeat
	metadata     map[string]string
	hostMetadata *types.HostMetadata
	assigned     *store.AssignedState
	statistics   map[string]store.ShardStatistics
}

type heartbeat struct {
	lastHeartbeat  time.Time
	status         types.ExecutorStatus
	reportedShards map[string]*types.ShardStatusReport
}

// changes records what a write touched, which decides the subscribers to signal.
type changes struct {
	// status is set when an executor status took a new value or was removed.
	status bool
	// visible is set when an assignment, executor metadata or the drained shard set changed.
	visible bool
}

func (c *changes) removeExecutor(executor *executorData) {
	c.status = c.status || executor.heartbeat != nil
	c.visible = c.visible || executor.assigned != nil || len(executor.metadata) > 0
}

// Stop closes every subscription and rejects new ones. It is idempotent.
func (s *Store) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return
	}
	s.stopped = true
	close(s.stopCh)
	for _, subs := range s.namespaceSubs {
		for _, ch := range subs {
			close(ch)
		}
	}
	s.namespaceSubs = nil
}

func (s *Store) RecordHeartbeat(_ context.Context, namespace, executorID string, state store.HeartbeatState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ns := s.namespace(namespace)
	executor := ns.executor(executorID)
	revision := s.commit()

	c := changes{status: executor.heartbeat == nil || executor.heartbeat.status != state.Status}
	executor.heartbeat = &heartbeat{
		lastHeartbeat:  state.LastHeartbeat,
		status:         state.Status,
		reportedShards: clonePointerMap(state.ReportedShards),
	}

	// Metadata is merged: keys a heartbeat leaves out keep their values.
	for key, value := range state.Metadata {
		if current, ok := executor.metadata[key]; ok && current == value {
			continue
		}
		if executor.metadata == nil {
			executor.metadata = make(map[string]string)
		}
		executor.metadata[key] = value
		c.visible = true
	}
	if state.HostMetadata != nil {
		executor.hostMetadata = clonePointer(state.HostMetadata)
	}

	// Metadata reaches spectators, so changing it moves the assignment version.
	if c.visible {
		ns.assignmentRevision = revision
	}
	s.publish(namespace, revision, c)
	return nil
}

// RecordShardStatistics writes statistics only if the executor's assignment is still
// at assignmentModRevision; zero means the executor has no assignment.
func (s *Store) RecordShardStatistics(
	_ context.Context,
	namespace string,
	executorID string,
	assignmentModRevision int64,
	statistics map[string]store.ShardStatistics,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ns := s.namespace(namespace)
	if ns.executors[executorID].assignedModRevision() != assignmentModRevision {
		return fmt.Errorf("%w: executor assignment changed", store.ErrVersionConflict)
	}
	s.commit()
	ns.executor(executorID).statistics = cloneOrEmpty(statistics)
	return nil
}

func (s *Store) RecordShardStatisticsBatch(_ context.Context, namespace string, updates []store.ExecutorShardStatistics) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ns := s.namespace(namespace)
	for _, update := range updates {
		if len(update.Statistics) == 0 {
			if executor, ok := ns.executors[update.ExecutorID]; ok && executor.statistics != nil {
				s.commit()
				executor.statistics = nil
				ns.prune(update.ExecutorID)
			}
			continue
		}
		s.commit()
		ns.executor(update.ExecutorID).statistics = maps.Clone(update.Statistics)
	}
	return nil
}

func (s *Store) GetExecutorState(_ context.Context, namespace string, executorID string) (store.ExecutorState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	executor, ok := s.read(namespace).executors[executorID]
	if !ok {
		return store.ExecutorState{}, store.ErrExecutorNotFound
	}

	heartbeatState := executor.heartbeatState()
	var assigned *store.AssignedState
	if executor.assigned != nil {
		clone := cloneAssignedState(*executor.assigned)
		assigned = &clone
	}
	return store.ExecutorState{
		Heartbeat:  &heartbeatState,
		Assignment: assigned,
		Statistics: cloneOrEmpty(executor.statistics),
	}, nil
}

func (s *Store) GetState(_ context.Context, namespace string) (*store.NamespaceState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ns := s.read(namespace)
	state := &store.NamespaceState{
		AssignmentState: s.assignmentState(ns),
		Executors:       make(map[string]store.HeartbeatState, len(ns.executors)),
		ShardStats:      make(map[string]store.ShardStatistics),
		DrainedHosts:    cloneOrEmpty(ns.drainedHosts),
	}
	for executorID, executor := range ns.executors {
		state.Executors[executorID] = executor.heartbeatState()
		for shardID, statistics := range executor.statistics {
			state.ShardStats[shardID] = statistics
		}
	}
	return state, nil
}

func (s *Store) GetAssignmentState(_ context.Context, namespace string) (*store.AssignmentState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.assignmentState(s.read(namespace))
	return &state, nil
}

func (s *Store) AssignShards(_ context.Context, namespace string, request store.AssignShardsRequest, guard store.GuardFunc) error {
	writes := make(map[string]store.AssignedState, len(request.NewState.ShardAssignments))
	for executorID, state := range request.NewState.ShardAssignments {
		if _, changed := request.ChangedExecutors[executorID]; request.ChangedExecutors == nil || changed {
			writes[executorID] = state
		}
	}
	if len(writes) == 0 && len(request.ExecutorsToDelete) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	held, err := checkGuard(guard)
	if err != nil {
		return err
	}
	if !held {
		return fmt.Errorf("%w: %w", store.ErrVersionConflict, errGuardFailed)
	}

	// Every executor in the new state is compared, not just the written ones, so the
	// plan is rejected if anything it was computed from has moved.
	ns := s.namespace(namespace)
	var conflicts []string
	checkRevision := func(executorID string, expected int64) {
		if actual := ns.executors[executorID].assignedModRevision(); actual != expected {
			conflicts = append(conflicts, fmt.Sprintf("{ executor: %s, expected: %d, actual: %d }", executorID, expected, actual))
		}
	}
	for executorID, expected := range request.ExecutorsToDelete {
		checkRevision(executorID, expected)
	}
	for executorID, state := range request.NewState.ShardAssignments {
		checkRevision(executorID, state.ModRevision)
	}
	if len(conflicts) > 0 {
		slices.Sort(conflicts)
		return fmt.Errorf("%w: transaction failed, a shard may have been concurrently assigned, %s",
			store.ErrVersionConflict, strings.Join(conflicts, ", "))
	}

	revision := s.commit()
	var c changes
	for executorID := range request.ExecutorsToDelete {
		if executor, ok := ns.executors[executorID]; ok {
			c.removeExecutor(executor)
			delete(ns.executors, executorID)
		}
	}
	for executorID, state := range writes {
		executor := ns.executor(executorID)
		assigned := cloneAssignedState(state)
		// A false mismatch costs subscribers one extra read; a false match would hide
		// a change, so plain deep equality is the safe comparison.
		if executor.assigned == nil || !sameAssignment(*executor.assigned, assigned) {
			c.visible = true
		}
		assigned.ModRevision = revision
		executor.assigned = &assigned
	}
	ns.assignmentRevision = revision
	s.publish(namespace, revision, c)
	return nil
}

// DeleteExecutors removes every record of the given executors. The shards they own are
// left for the leader to reassign.
func (s *Store) DeleteExecutors(_ context.Context, namespace string, executorIDs []string, guard store.GuardFunc) error {
	if len(executorIDs) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := requireGuard(guard); err != nil {
		return fmt.Errorf("delete executors: %w", err)
	}

	ns := s.namespace(namespace)
	revision := s.commit()
	var c changes
	for _, executorID := range executorIDs {
		if executor, ok := ns.executors[executorID]; ok {
			c.removeExecutor(executor)
			delete(ns.executors, executorID)
		}
	}
	ns.assignmentRevision = revision
	s.publish(namespace, revision, c)
	return nil
}

func (s *Store) DeleteShardStats(_ context.Context, namespace string, shardIDs []string, guard store.GuardFunc) error {
	if len(shardIDs) == 0 {
		return nil
	}
	toDelete := make(map[string]struct{}, len(shardIDs))
	for _, shardID := range shardIDs {
		toDelete[shardID] = struct{}{}
	}
	isDeleted := func(shardID string, _ store.ShardStatistics) bool {
		_, ok := toDelete[shardID]
		return ok
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ns := s.namespace(namespace)
	var affected []string
	for executorID, executor := range ns.executors {
		for shardID := range executor.statistics {
			if _, ok := toDelete[shardID]; ok {
				affected = append(affected, executorID)
				break
			}
		}
	}
	if len(affected) == 0 {
		return nil
	}

	if err := requireGuard(guard); err != nil {
		return fmt.Errorf("delete shard stats: %w", err)
	}

	s.commit()
	for _, executorID := range affected {
		executor := ns.executors[executorID]
		maps.DeleteFunc(executor.statistics, isDeleted)
		if len(executor.statistics) == 0 {
			executor.statistics = nil
			ns.prune(executorID)
		}
	}
	return nil
}

// ResetNamespace removes all of the namespace's records. The count it returns follows
// the etcd store's key layout, so reset output reads the same on either backend.
func (s *Store) ResetNamespace(_ context.Context, namespace string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ns := s.read(namespace)
	c := changes{visible: len(ns.drainedShards) > 0}
	removed := int64(len(ns.drainedShards) + len(ns.drainedHosts))
	if ns.assignmentRevision > 0 {
		removed++
	}
	for _, executor := range ns.executors {
		c.removeExecutor(executor)
		removed += executor.recordCount()
	}

	revision := s.commit()
	reset := newNamespaceData()
	reset.assignmentRevision = revision
	s.namespaces[namespace] = reset
	s.publish(namespace, revision, c)
	return removed, nil
}

// DrainShards is not guarded by leadership: draining is an operator action that must
// work whichever host leads, and the leader picks it up on its next rebalance.
func (s *Store) DrainShards(_ context.Context, namespace string, shardIDs []string) error {
	if len(shardIDs) == 0 {
		return nil
	}
	for _, shardID := range shardIDs {
		if err := validateShardID(shardID); err != nil {
			return fmt.Errorf("drain shards: %w", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ns := s.namespace(namespace)
	revision := s.commit()
	for _, shardID := range shardIDs {
		ns.drainedShards[shardID] = struct{}{}
	}
	ns.assignmentRevision = revision
	s.publish(namespace, revision, changes{visible: true})
	return nil
}

func (s *Store) UndrainShards(_ context.Context, namespace string, shardIDs []string) ([]string, error) {
	if len(shardIDs) == 0 {
		return nil, nil
	}
	for _, shardID := range shardIDs {
		if err := validateShardID(shardID); err != nil {
			return nil, fmt.Errorf("undrain shards: %w", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ns := s.namespace(namespace)
	revision := s.commit()
	removed := make([]string, 0, len(shardIDs))
	for _, shardID := range shardIDs {
		if _, ok := ns.drainedShards[shardID]; ok {
			delete(ns.drainedShards, shardID)
			removed = append(removed, shardID)
		}
	}
	ns.assignmentRevision = revision
	s.publish(namespace, revision, changes{visible: len(removed) > 0})
	return removed, nil
}

func (s *Store) GetDrainedShards(_ context.Context, namespace string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Sorted(maps.Keys(s.read(namespace).drainedShards)), nil
}

func (s *Store) DrainHosts(_ context.Context, namespace string, hosts []store.DrainedHost) error {
	if len(hosts) == 0 {
		return nil
	}
	validated, err := validateDrainedHosts(hosts, s.timeSource.Now().UTC())
	if err != nil {
		return fmt.Errorf("drain hosts: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ns := s.namespace(namespace)
	maps.DeleteFunc(validated, func(name string, _ store.DrainedHost) bool {
		_, already := ns.drainedHosts[name]
		return already
	})
	if len(validated) == 0 {
		return nil
	}
	s.commit()
	maps.Copy(ns.drainedHosts, validated)
	return nil
}

func (s *Store) UndrainHosts(_ context.Context, namespace string, hostnames []string) ([]string, error) {
	if len(hostnames) == 0 {
		return nil, nil
	}
	for _, name := range hostnames {
		if err := hostname.Validate(name); err != nil {
			return nil, fmt.Errorf("undrain hosts: %w", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ns := s.namespace(namespace)
	removed := make([]string, 0, len(hostnames))
	for _, name := range hostnames {
		if _, ok := ns.drainedHosts[name]; ok {
			delete(ns.drainedHosts, name)
			removed = append(removed, name)
		}
	}
	if len(removed) > 0 {
		s.commit()
	}
	return removed, nil
}

func (s *Store) GetDrainedHosts(_ context.Context, namespace string) ([]store.DrainedHost, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	drained := s.read(namespace).drainedHosts
	hosts := make([]store.DrainedHost, 0, len(drained))
	for _, name := range slices.Sorted(maps.Keys(drained)) {
		hosts = append(hosts, drained[name])
	}
	return hosts, nil
}

// ListRanges returns no ranges: AssignRanges, the only way to create one, is not
// implemented yet.
func (s *Store) ListRanges(context.Context, string) ([]*store.RangeAssignment, error) {
	return []*store.RangeAssignment{}, nil
}

func (s *Store) AssignRanges(context.Context, string, store.AssignRangesRequest, store.GuardFunc) error {
	return store.ErrRangeNotImplemented
}

// SubscribeToExecutorStatusChanges sends the store revision of every write that changes
// an executor status. A slow reader sees only the latest revision. The channel is
// closed when ctx ends or the store stops.
func (s *Store) SubscribeToExecutorStatusChanges(ctx context.Context, namespace string) (<-chan int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return nil, errStopped
	}
	ch := make(chan int64, 1)
	if s.statusSubs[namespace] == nil {
		s.statusSubs[namespace] = make(map[chan int64]struct{})
	}
	s.statusSubs[namespace][ch] = struct{}{}

	go func() {
		select {
		case <-ctx.Done():
		case <-s.stopCh:
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.statusSubs[namespace], ch)
		close(ch)
	}()
	return ch, nil
}

func (s *Store) SubscribeToNamespaceChanges(namespace string) (<-chan struct{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return nil, errStopped
	}
	// Signalled up front so the subscriber reads the state once before it waits.
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	s.namespaceSubs[namespace] = append(s.namespaceSubs[namespace], ch)
	return ch, nil
}

// publish signals the subscribers a write concerns. Sends never block, and holding the
// lock keeps them from racing the close of a channel.
func (s *Store) publish(namespace string, revision int64, c changes) {
	if c.visible {
		for _, ch := range s.namespaceSubs[namespace] {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}
	if c.status {
		for ch := range s.statusSubs[namespace] {
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- revision:
			default:
			}
		}
	}
}

// commit advances the store revision for a write and returns the new value.
func (s *Store) commit() int64 {
	s.revision++
	return s.revision
}

// namespace returns the namespace's data for writing, creating it if needed.
func (s *Store) namespace(name string) *namespaceData {
	ns, ok := s.namespaces[name]
	if !ok {
		ns = newNamespaceData()
		s.namespaces[name] = ns
	}
	return ns
}

// read returns the namespace's data for reading, without creating it.
func (s *Store) read(name string) *namespaceData {
	if ns, ok := s.namespaces[name]; ok {
		return ns
	}
	return &namespaceData{}
}

func (s *Store) assignmentState(ns *namespaceData) store.AssignmentState {
	state := store.AssignmentState{
		ExecutorMetadata:   make(map[string]map[string]string, len(ns.executors)),
		ShardAssignments:   make(map[string]store.AssignedState),
		DrainedShards:      cloneOrEmpty(ns.drainedShards),
		Revision:           s.revision,
		AssignmentRevision: ns.assignmentRevision,
	}
	for executorID, executor := range ns.executors {
		state.ExecutorMetadata[executorID] = cloneOrEmpty(executor.metadata)
		if executor.assigned != nil {
			state.ShardAssignments[executorID] = cloneAssignedState(*executor.assigned)
		}
	}
	return state
}

func newNamespaceData() *namespaceData {
	return &namespaceData{
		executors:     make(map[string]*executorData),
		drainedShards: make(map[string]struct{}),
		drainedHosts:  make(map[string]store.DrainedHost),
	}
}

func (ns *namespaceData) executor(executorID string) *executorData {
	executor, ok := ns.executors[executorID]
	if !ok {
		executor = &executorData{}
		ns.executors[executorID] = executor
	}
	return executor
}

// prune drops an executor once its last record is gone.
func (ns *namespaceData) prune(executorID string) {
	executor := ns.executors[executorID]
	if executor.heartbeat == nil && len(executor.metadata) == 0 && executor.hostMetadata == nil &&
		executor.assigned == nil && executor.statistics == nil {
		delete(ns.executors, executorID)
	}
}

func (e *executorData) assignedModRevision() int64 {
	if e == nil || e.assigned == nil {
		return 0
	}
	return e.assigned.ModRevision
}

// recordCount counts the executor's records as etcd keys.
func (e *executorData) recordCount() int64 {
	count := int64(len(e.metadata))
	if e.heartbeat != nil {
		// Heartbeat time, status and reported shards.
		count += 3
	}
	for _, present := range []bool{e.hostMetadata != nil, e.assigned != nil, e.statistics != nil} {
		if present {
			count++
		}
	}
	return count
}

func (e *executorData) heartbeatState() store.HeartbeatState {
	state := store.HeartbeatState{
		ReportedShards: map[string]*types.ShardStatusReport{},
		Metadata:       cloneOrEmpty(e.metadata),
		HostMetadata:   clonePointer(e.hostMetadata),
	}
	if e.heartbeat != nil {
		state.LastHeartbeat = e.heartbeat.lastHeartbeat
		state.Status = e.heartbeat.status
		state.ReportedShards = clonePointerMap(e.heartbeat.reportedShards)
	}
	return state
}

func sameAssignment(a, b store.AssignedState) bool {
	a.ModRevision, b.ModRevision = 0, 0
	return reflect.DeepEqual(a, b)
}

// validateShardID rejects what the etcd store rejects, so state written here stays
// valid if a namespace moves to etcd.
func validateShardID(shardID string) error {
	if shardID == "" {
		return errors.New("shard ID must not be empty")
	}
	if strings.Contains(shardID, "/") {
		return fmt.Errorf("shard ID '%s' must not contain '/'", shardID)
	}
	return nil
}

// validateDrainedHosts rejects unusable hostnames and collapses duplicates, keeping the
// first occurrence of each host.
func validateDrainedHosts(hosts []store.DrainedHost, now time.Time) (map[string]store.DrainedHost, error) {
	validated := make(map[string]store.DrainedHost, len(hosts))
	for _, host := range hosts {
		if err := hostname.Validate(host.Hostname); err != nil {
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

// cloneOrEmpty returns a copy that is never nil, matching what decoding a stored record yields.
func cloneOrEmpty[M ~map[K]V, K comparable, V any](m M) M {
	if m == nil {
		return M{}
	}
	return maps.Clone(m)
}

func clonePointer[T any](v *T) *T {
	if v == nil {
		return nil
	}
	clone := *v
	return &clone
}

// clonePointerMap copies the map and the values it points to.
func clonePointerMap[V any](m map[string]*V) map[string]*V {
	clone := make(map[string]*V, len(m))
	for key, value := range m {
		clone[key] = clonePointer(value)
	}
	return clone
}

func cloneAssignedState(state store.AssignedState) store.AssignedState {
	state.AssignedShards = clonePointerMap(state.AssignedShards)
	state.ShardHandoverStats = maps.Clone(state.ShardHandoverStats)
	return state
}
