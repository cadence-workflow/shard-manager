package store

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cadence-workflow/shard-manager/common/types"
)

func TestNamespaceState_CountExecutorsByStatus(t *testing.T) {
	tests := []struct {
		name      string
		executors map[string]HeartbeatState
		expected  map[types.ExecutorStatus]int
	}{
		{
			name:      "empty executors",
			executors: map[string]HeartbeatState{},
			expected:  map[types.ExecutorStatus]int{},
		},
		{
			name: "single active executor",
			executors: map[string]HeartbeatState{
				"exec-1": {Status: types.ExecutorStatusACTIVE},
			},
			expected: map[types.ExecutorStatus]int{
				types.ExecutorStatusACTIVE: 1,
			},
		},
		{
			name: "multiple executors same status",
			executors: map[string]HeartbeatState{
				"exec-1": {Status: types.ExecutorStatusACTIVE},
				"exec-2": {Status: types.ExecutorStatusACTIVE},
				"exec-3": {Status: types.ExecutorStatusACTIVE},
			},
			expected: map[types.ExecutorStatus]int{
				types.ExecutorStatusACTIVE: 3,
			},
		},
		{
			name: "all statuses",
			executors: map[string]HeartbeatState{
				"exec-1": {Status: types.ExecutorStatusINVALID},
				"exec-2": {Status: types.ExecutorStatusACTIVE},
				"exec-3": {Status: types.ExecutorStatusDRAINING},
				"exec-4": {Status: types.ExecutorStatusDRAINED},
			},
			expected: map[types.ExecutorStatus]int{
				types.ExecutorStatusINVALID:  1,
				types.ExecutorStatusACTIVE:   1,
				types.ExecutorStatusDRAINING: 1,
				types.ExecutorStatusDRAINED:  1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := &NamespaceState{
				Executors: tt.executors,
			}
			result := ns.CountExecutorsByStatus()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNamespaceState_ShardOwners(t *testing.T) {
	ready := func(shards ...string) AssignedState {
		assigned := make(map[string]*types.ShardAssignment, len(shards))
		for _, shard := range shards {
			assigned[shard] = &types.ShardAssignment{Status: types.AssignmentStatusREADY}
		}
		return AssignedState{AssignedShards: assigned}
	}

	tests := []struct {
		name             string
		executors        map[string]HeartbeatState
		shardAssignments map[string]AssignedState
		expected         map[string]string
	}{
		{
			name:     "empty state",
			expected: map[string]string{},
		},
		{
			name: "all assignments are flattened",
			executors: map[string]HeartbeatState{
				"exec-1": {Status: types.ExecutorStatusACTIVE},
				"exec-2": {Status: types.ExecutorStatusACTIVE},
			},
			shardAssignments: map[string]AssignedState{
				"exec-1": ready("shard-1", "shard-2"),
				"exec-2": ready("shard-3"),
			},
			expected: map[string]string{
				"shard-1": "exec-1",
				"shard-2": "exec-1",
				"shard-3": "exec-2",
			},
		},
		{
			name: "shards of draining and drained executors are included",
			executors: map[string]HeartbeatState{
				"exec-active":   {Status: types.ExecutorStatusACTIVE},
				"exec-draining": {Status: types.ExecutorStatusDRAINING},
				"exec-drained":  {Status: types.ExecutorStatusDRAINED},
			},
			shardAssignments: map[string]AssignedState{
				"exec-active":   ready("shard-1"),
				"exec-draining": ready("shard-2"),
				"exec-drained":  ready("shard-3"),
			},
			expected: map[string]string{
				"shard-1": "exec-active",
				"shard-2": "exec-draining",
				"shard-3": "exec-drained",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := &NamespaceState{
				AssignmentState: AssignmentState{
					ShardAssignments: tt.shardAssignments,
				},
				Executors: tt.executors}
			assert.Equal(t, tt.expected, ns.ShardOwners())
		})
	}
}

func TestNamespaceState_Executor(t *testing.T) {
	heartbeat := HeartbeatState{
		Status:       types.ExecutorStatusACTIVE,
		HostMetadata: &types.HostMetadata{HostName: "host-a"},
	}
	otherHost := HeartbeatState{
		Status:       types.ExecutorStatusACTIVE,
		HostMetadata: &types.HostMetadata{HostName: "host-b"},
	}
	ns := &NamespaceState{
		AssignmentState: AssignmentState{
			ShardAssignments: map[string]AssignedState{
				"assigned-only": {AssignedShards: map[string]*types.ShardAssignment{"shard-1": {}}},
			},
		},
		Executors: map[string]HeartbeatState{
			"host-b@uuid": heartbeat,
			"host-a@uuid": otherHost,
		},
		DrainedHosts: map[string]DrainedHost{"host-a": {Hostname: "host-a"}},
	}

	tests := []struct {
		name         string
		id           string
		want         Executor
		wantOK       bool
		wantHostname string
	}{
		{
			name:         "hostname comes from host metadata, not executor id",
			id:           "host-b@uuid",
			want:         Executor{ID: "host-b@uuid", Heartbeat: heartbeat, HostDrained: true},
			wantOK:       true,
			wantHostname: "host-a",
		},
		{
			name:         "executor id naming a drained host is ignored",
			id:           "host-a@uuid",
			want:         Executor{ID: "host-a@uuid", Heartbeat: otherHost},
			wantOK:       true,
			wantHostname: "host-b",
		},
		{name: "no heartbeat", id: "assigned-only"},
		{name: "missing", id: "missing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ns.Executor(tt.id)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantHostname, got.Hostname())
		})
	}
}

func TestNamespaceState_AssignableExecutorIDs(t *testing.T) {
	state := &NamespaceState{
		Executors: map[string]HeartbeatState{
			"exec-c":        {Status: types.ExecutorStatusACTIVE},
			"exec-a":        {Status: types.ExecutorStatusACTIVE},
			"exec-b":        {Status: types.ExecutorStatusACTIVE},
			"draining":      {Status: types.ExecutorStatusDRAINING},
			"stale":         {Status: types.ExecutorStatusACTIVE},
			"host-a@uuid-1": {Status: types.ExecutorStatusACTIVE, HostMetadata: &types.HostMetadata{HostName: "host-a"}},
		},
		DrainedHosts: map[string]DrainedHost{"host-a": {Hostname: "host-a"}},
	}

	assert.Equal(t, []string{"exec-a", "exec-b", "exec-c"}, state.AssignableExecutorIDs(map[string]int64{"stale": 1}))
}

func TestExecutor_IsAssignable(t *testing.T) {
	staleExecutors := map[string]int64{"stale": 1}

	tests := []struct {
		name     string
		executor Executor
		want     bool
	}{
		{name: "active", executor: Executor{ID: "active", Heartbeat: HeartbeatState{Status: types.ExecutorStatusACTIVE}}, want: true},
		{name: "draining", executor: Executor{ID: "draining", Heartbeat: HeartbeatState{Status: types.ExecutorStatusDRAINING}}, want: false},
		{name: "drained", executor: Executor{ID: "drained", Heartbeat: HeartbeatState{Status: types.ExecutorStatusDRAINED}}, want: false},
		{name: "invalid status", executor: Executor{ID: "invalid"}, want: false},
		{name: "stale", executor: Executor{ID: "stale", Heartbeat: HeartbeatState{Status: types.ExecutorStatusACTIVE}}, want: false},
		{name: "active on drained host", executor: Executor{ID: "active", Heartbeat: HeartbeatState{Status: types.ExecutorStatusACTIVE}, HostDrained: true}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.executor.IsAssignable(staleExecutors))
		})
	}
}

func TestExecutor_IsDraining(t *testing.T) {
	tests := []struct {
		name     string
		executor Executor
		want     bool
	}{
		{name: "active", executor: Executor{Heartbeat: HeartbeatState{Status: types.ExecutorStatusACTIVE}}, want: false},
		{name: "draining", executor: Executor{Heartbeat: HeartbeatState{Status: types.ExecutorStatusDRAINING}}, want: true},
		{name: "drained", executor: Executor{Heartbeat: HeartbeatState{Status: types.ExecutorStatusDRAINED}}, want: true},
		{name: "invalid status", executor: Executor{}, want: false},
		{name: "active on drained host", executor: Executor{Heartbeat: HeartbeatState{Status: types.ExecutorStatusACTIVE}, HostDrained: true}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.executor.IsDraining())
		})
	}
}

func TestNamespaceState_IsHostDrained(t *testing.T) {
	tests := []struct {
		name     string
		hosts    map[string]DrainedHost
		hostname string
		want     bool
	}{
		{name: "nil map", hostname: "host-a", want: false},
		{name: "empty map", hosts: map[string]DrainedHost{}, hostname: "host-a", want: false},
		{
			name:     "drained",
			hosts:    map[string]DrainedHost{"host-a": {Hostname: "host-a"}},
			hostname: "host-a",
			want:     true,
		},
		{
			name:     "other host",
			hosts:    map[string]DrainedHost{"host-a": {Hostname: "host-a"}},
			hostname: "host-b",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := &NamespaceState{DrainedHosts: tt.hosts}
			assert.Equal(t, tt.want, ns.IsHostDrained(tt.hostname))
		})
	}
}
