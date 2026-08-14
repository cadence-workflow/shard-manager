package smctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	cliv3 "github.com/urfave/cli/v3"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/types"
)

func TestExecutorCommand_help_listsSubcommands(t *testing.T) {
	t.Parallel()
	cmd := BuildCommand()
	buf := new(bytes.Buffer)
	cmd.Writer = buf

	if err := cmd.Run(context.Background(), []string{"smctl", "executor", "--help"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"list", "state"} {
		if !strings.Contains(out, want) {
			t.Errorf("executor help should list %q subcommand:\n%s", want, out)
		}
	}
}

func TestGetExecutorState(t *testing.T) {
	heartbeatAt := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

	type setupResult struct {
		client    sharddistributor.Client
		clientErr error
	}

	tests := []struct {
		name    string
		args    []string
		setup   func(t *testing.T, ctrl *gomock.Controller) setupResult
		wantErr string
		check   func(t *testing.T, stdout string)
	}{
		{
			name: "success: prints complete executor state as JSON",
			args: []string{
				"smctl",
				"-n", "ns-1",
				"executor", "state",
				"--" + FlagExecutorID, "executor-a",
			},
			setup: func(t *testing.T, ctrl *gomock.Controller) setupResult {
				mc := sharddistributor.NewMockClient(ctrl)
				mc.EXPECT().
					GetExecutorState(gomock.Any(), &types.GetExecutorStateRequest{
						Namespace:  "ns-1",
						ExecutorID: "executor-a",
					}).
					Return(&types.GetExecutorStateResponse{
						Namespace: "ns-1",
						Executor: &types.NamespaceExecutorState{
							ExecutorID:    "executor-a",
							Status:        types.ExecutorStatusACTIVE,
							LastHeartbeat: heartbeatAt,
							Metadata:      map[string]string{"zone": "dca1"},
							AssignedShards: []*types.ExecutorAssignedShardState{
								{
									ShardKey:                 "shard-1",
									AssignmentStatus:         types.AssignmentStatusREADY,
									AssignedStateModRevision: 7,
								},
							},
						},
					}, nil)
				return setupResult{client: mc}
			},
			check: func(t *testing.T, stdout string) {
				var resp types.GetExecutorStateResponse
				if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
					t.Fatalf("output is not valid JSON: %v\nout: %s", err, stdout)
				}
				if resp.GetNamespace() != "ns-1" {
					t.Errorf("namespace: got %q want ns-1", resp.GetNamespace())
				}
				executor := resp.GetExecutor()
				if executor == nil {
					t.Fatal("executor should be present")
				}
				if executor.GetExecutorID() != "executor-a" {
					t.Errorf("executor ID: got %q want executor-a", executor.GetExecutorID())
				}
				if len(executor.GetAssignedShards()) != 1 || executor.GetAssignedShards()[0].GetShardKey() != "shard-1" {
					t.Errorf("assigned shards: got %+v want shard-1", executor.GetAssignedShards())
				}
				if !strings.Contains(stdout, "AssignmentStatusREADY") {
					t.Errorf("assignment status should be marshalled as enum string, got: %s", stdout)
				}
			},
		},
		{
			name: "aliases ex st and eid invoke executor state",
			args: []string{"smctl", "-n", "ns-1", "ex", "st", "--eid", "executor-a"},
			setup: func(t *testing.T, ctrl *gomock.Controller) setupResult {
				mc := sharddistributor.NewMockClient(ctrl)
				mc.EXPECT().
					GetExecutorState(gomock.Any(), &types.GetExecutorStateRequest{
						Namespace:  "ns-1",
						ExecutorID: "executor-a",
					}).
					Return(&types.GetExecutorStateResponse{Namespace: "ns-1"}, nil)
				return setupResult{client: mc}
			},
		},
		{
			name:    "missing namespace fails",
			args:    []string{"smctl", "executor", "state", "--" + FlagExecutorID, "executor-a"},
			setup:   func(t *testing.T, ctrl *gomock.Controller) setupResult { return setupResult{} },
			wantErr: "--" + FlagNamespace + " is required",
		},
		{
			name:    "missing executor ID fails",
			args:    []string{"smctl", "-n", "ns-1", "executor", "state"},
			setup:   func(t *testing.T, ctrl *gomock.Controller) setupResult { return setupResult{} },
			wantErr: "--" + FlagExecutorID + " is required",
		},
		{
			name: "API error surfaces EntityNotExists",
			args: []string{
				"smctl",
				"-n", "ns-1",
				"executor", "state",
				"--" + FlagExecutorID, "missing",
			},
			setup: func(t *testing.T, ctrl *gomock.Controller) setupResult {
				mc := sharddistributor.NewMockClient(ctrl)
				mc.EXPECT().
					GetExecutorState(gomock.Any(), &types.GetExecutorStateRequest{
						Namespace:  "ns-1",
						ExecutorID: "missing",
					}).
					Return(nil, &types.EntityNotExistsError{Message: "executor not found ns-1:missing"})
				return setupResult{client: mc}
			},
			wantErr: "GetExecutorState: executor not found ns-1:missing",
		},
		{
			name: "factory error is propagated",
			args: []string{
				"smctl",
				"-n", "ns-1",
				"executor", "state",
				"--" + FlagExecutorID, "executor-a",
			},
			setup: func(t *testing.T, ctrl *gomock.Controller) setupResult {
				return setupResult{clientErr: errors.New("dial: refused")}
			},
			wantErr: "dial: refused",
		},
		{
			name: "context timeout flag is honored",
			args: []string{
				"smctl",
				"--" + FlagContextTimeout, "1ms",
				"-n", "ns-1",
				"executor", "state",
				"--" + FlagExecutorID, "executor-a",
			},
			setup: func(t *testing.T, ctrl *gomock.Controller) setupResult {
				mc := sharddistributor.NewMockClient(ctrl)
				mc.EXPECT().
					GetExecutorState(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, _ *types.GetExecutorStateRequest, _ ...any) (*types.GetExecutorStateResponse, error) {
						deadline, ok := ctx.Deadline()
						if !ok {
							t.Errorf("ctx should have deadline from --%s", FlagContextTimeout)
							return &types.GetExecutorStateResponse{}, nil
						}
						if remaining := time.Until(deadline); remaining > 50*time.Millisecond {
							t.Errorf("--%s=1ms should produce a sub-50ms deadline, got %v", FlagContextTimeout, remaining)
						}
						return &types.GetExecutorStateResponse{Namespace: "ns-1"}, nil
					})
				return setupResult{client: mc}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			res := tt.setup(t, ctrl)

			cf := NewMockClientFactory(ctrl)
			cf.EXPECT().Close().Return(nil).Times(1)
			if tt.wantErr == "" || res.client != nil || res.clientErr != nil {
				cf.EXPECT().
					ShardManagerClient(gomock.Any()).
					Return(res.client, res.clientErr).
					MaxTimes(1)
			}

			cmd := BuildCommandWithFactory(cf)
			buf := new(bytes.Buffer)
			cmd.Writer = buf
			cmd.ErrWriter = buf

			err := cmd.Run(context.Background(), tt.args)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil. out=%s", tt.wantErr, buf.String())
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error: got %q want substring %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}

func TestListExecutors(t *testing.T) {
	heartbeatAt := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)

	twoExecutors := &types.GetNamespaceStateResponse{
		Namespace: "ns-1",
		Executors: []*types.NamespaceExecutorState{
			{
				ExecutorID:    "executor-b",
				Status:        types.ExecutorStatusDRAINING,
				LastHeartbeat: heartbeatAt,
				Metadata:      map[string]string{"zone": "dca1"},
				AssignedShards: []*types.ExecutorAssignedShardState{
					{ShardKey: "shard-3", AssignmentStatus: types.AssignmentStatusREADY},
				},
			},
			{
				ExecutorID:    "executor-a",
				Status:        types.ExecutorStatusACTIVE,
				LastHeartbeat: heartbeatAt,
				Metadata:      map[string]string{"zone": "phx1", "ip": "10.0.0.1"},
				AssignedShards: []*types.ExecutorAssignedShardState{
					{ShardKey: "shard-1", AssignmentStatus: types.AssignmentStatusREADY},
					{ShardKey: "shard-2", AssignmentStatus: types.AssignmentStatusREADY},
				},
			},
		},
	}

	type setupResult struct {
		client     sharddistributor.Client
		clientErr  error
		expectArgs func(t *testing.T, cmd *cliv3.Command)
	}

	tests := []struct {
		name    string
		args    []string
		setup   func(t *testing.T, ctrl *gomock.Controller) setupResult
		wantErr string
		check   func(t *testing.T, stdout string)
	}{
		{
			name: "table output sorts by executor id and renders headers + columns",
			args: []string{"smctl", "-n", "ns-1", "executor", "list"},
			setup: func(t *testing.T, ctrl *gomock.Controller) setupResult {
				mc := sharddistributor.NewMockClient(ctrl)
				mc.EXPECT().
					GetNamespaceState(gomock.Any(), &types.GetNamespaceStateRequest{Namespace: "ns-1"}).
					Return(twoExecutors, nil)
				return setupResult{client: mc}
			},
			check: func(t *testing.T, stdout string) {
				lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
				if len(lines) != 3 {
					t.Fatalf("expected 3 lines (header + 2 rows), got %d:\n%s", len(lines), stdout)
				}
				for _, want := range []string{"EXECUTOR_ID", "STATUS", "LAST_HEARTBEAT", "SHARDS", "METADATA"} {
					if !strings.Contains(lines[0], want) {
						t.Errorf("header missing %q column: %q", want, lines[0])
					}
				}
				// Sorted by ExecutorID -> executor-a comes before executor-b.
				if !strings.HasPrefix(lines[1], "executor-a") {
					t.Errorf("expected executor-a first, got: %q", lines[1])
				}
				if !strings.HasPrefix(lines[2], "executor-b") {
					t.Errorf("expected executor-b second, got: %q", lines[2])
				}
				// Status enum prefix is stripped in the table view.
				if !strings.Contains(lines[1], "ACTIVE") {
					t.Errorf("executor-a row should show short status ACTIVE, got: %q", lines[1])
				}
				if strings.Contains(lines[1], "ExecutorStatusACTIVE") {
					t.Errorf("table should not contain enum prefix, got: %q", lines[1])
				}
				// Shard counts are correct.
				if !strings.Contains(lines[1], " 2 ") {
					t.Errorf("executor-a should show 2 shards, got: %q", lines[1])
				}
				if !strings.Contains(lines[2], " 1 ") {
					t.Errorf("executor-b should show 1 shard, got: %q", lines[2])
				}
				// Metadata renders sorted "k1=v1,k2=v2".
				if !strings.Contains(lines[1], "ip=10.0.0.1,zone=phx1") {
					t.Errorf("executor-a metadata should be sorted by key, got: %q", lines[1])
				}
				// Heartbeat renders as RFC3339 UTC.
				if !strings.Contains(lines[1], "2026-05-07T12:00:00Z") {
					t.Errorf("executor-a heartbeat should be RFC3339 UTC, got: %q", lines[1])
				}
			},
		},
		{
			name: "--json flag emits envelope with namespace and projected summaries",
			args: []string{"smctl", "-n", "ns-1", "executor", "list", "--json"},
			setup: func(t *testing.T, ctrl *gomock.Controller) setupResult {
				mc := sharddistributor.NewMockClient(ctrl)
				mc.EXPECT().
					GetNamespaceState(gomock.Any(), &types.GetNamespaceStateRequest{Namespace: "ns-1"}).
					Return(twoExecutors, nil)
				return setupResult{client: mc}
			},
			check: func(t *testing.T, stdout string) {
				var envelope struct {
					Namespace string `json:"namespace"`
					Executors []struct {
						ExecutorID    string            `json:"executorId"`
						Status        string            `json:"status"`
						LastHeartbeat time.Time         `json:"lastHeartbeat"`
						ShardCount    int               `json:"shardCount"`
						Metadata      map[string]string `json:"metadata,omitempty"`
					} `json:"executors"`
				}
				if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
					t.Fatalf("output is not valid JSON: %v\nout: %s", err, stdout)
				}
				if envelope.Namespace != "ns-1" {
					t.Errorf("namespace: got %q want %q", envelope.Namespace, "ns-1")
				}
				if len(envelope.Executors) != 2 {
					t.Fatalf("expected 2 executors in JSON, got %d: %s", len(envelope.Executors), stdout)
				}
				// Same sort as table.
				if envelope.Executors[0].ExecutorID != "executor-a" {
					t.Errorf("executor[0]: got %q want executor-a", envelope.Executors[0].ExecutorID)
				}
				if envelope.Executors[0].ShardCount != 2 {
					t.Errorf("executor-a shardCount: got %d want 2", envelope.Executors[0].ShardCount)
				}
				if envelope.Executors[0].Status != "ACTIVE" {
					t.Errorf("executor-a status: got %q want ACTIVE", envelope.Executors[0].Status)
				}
				// Per-shard detail must NOT leak into the summary.
				if strings.Contains(stdout, "shard-1") || strings.Contains(stdout, "shard-2") || strings.Contains(stdout, "shard-3") {
					t.Errorf("summary should not include individual shard keys, got:\n%s", stdout)
				}
			},
		},
		{
			name: "alias 'ex ls' invokes executor list",
			args: []string{"smctl", "-n", "ns-1", "ex", "ls"},
			setup: func(t *testing.T, ctrl *gomock.Controller) setupResult {
				mc := sharddistributor.NewMockClient(ctrl)
				mc.EXPECT().
					GetNamespaceState(gomock.Any(), &types.GetNamespaceStateRequest{Namespace: "ns-1"}).
					Return(&types.GetNamespaceStateResponse{Namespace: "ns-1"}, nil)
				return setupResult{client: mc}
			},
			check: func(t *testing.T, stdout string) {
				if !strings.Contains(stdout, "EXECUTOR_ID") {
					t.Errorf("empty response should still print header row, got:\n%s", stdout)
				}
			},
		},
		{
			name:    "missing --namespace fails with required-flag error",
			args:    []string{"smctl", "executor", "list"},
			setup:   func(t *testing.T, ctrl *gomock.Controller) setupResult { return setupResult{} },
			wantErr: `--` + FlagNamespace + ` is required`,
		},
		{
			name: "API error surfaces with operation prefix",
			args: []string{"smctl", "-n", "ns-1", "executor", "list"},
			setup: func(t *testing.T, ctrl *gomock.Controller) setupResult {
				mc := sharddistributor.NewMockClient(ctrl)
				mc.EXPECT().
					GetNamespaceState(gomock.Any(), &types.GetNamespaceStateRequest{Namespace: "ns-1"}).
					Return(nil, errors.New("rpc unavailable"))
				return setupResult{client: mc}
			},
			wantErr: "GetNamespaceState: rpc unavailable",
		},
		{
			name: "API error surfaces NamespaceNotFound",
			args: []string{"smctl", "-n", "missing", "executor", "list"},
			setup: func(t *testing.T, ctrl *gomock.Controller) setupResult {
				mc := sharddistributor.NewMockClient(ctrl)
				mc.EXPECT().
					GetNamespaceState(gomock.Any(), &types.GetNamespaceStateRequest{Namespace: "missing"}).
					Return(nil, &types.NamespaceNotFoundError{Namespace: "missing"})
				return setupResult{client: mc}
			},
			wantErr: "namespace not found missing",
		},
		{
			name: "factory error short-circuits before RPC",
			args: []string{"smctl", "-n", "ns-1", "executor", "list"},
			setup: func(t *testing.T, ctrl *gomock.Controller) setupResult {
				return setupResult{clientErr: errors.New("dial: refused")}
			},
			wantErr: "dial: refused",
		},
		{
			name: "context-timeout flag is honored",
			args: []string{
				"smctl",
				"--" + FlagContextTimeout, "1ms",
				"-n", "ns-1",
				"executor", "list",
			},
			setup: func(t *testing.T, ctrl *gomock.Controller) setupResult {
				mc := sharddistributor.NewMockClient(ctrl)
				mc.EXPECT().
					GetNamespaceState(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, _ *types.GetNamespaceStateRequest, _ ...any) (*types.GetNamespaceStateResponse, error) {
						deadline, ok := ctx.Deadline()
						if !ok {
							t.Errorf("ctx should have deadline from --%s", FlagContextTimeout)
							return &types.GetNamespaceStateResponse{}, nil
						}
						if remaining := time.Until(deadline); remaining > 50*time.Millisecond {
							t.Errorf("--%s=1ms should produce a sub-50ms deadline, got %v", FlagContextTimeout, remaining)
						}
						return &types.GetNamespaceStateResponse{Namespace: "ns-1"}, nil
					})
				return setupResult{client: mc}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			res := tt.setup(t, ctrl)

			cf := NewMockClientFactory(ctrl)
			cf.EXPECT().Close().Return(nil).Times(1)
			if tt.wantErr == "" || res.client != nil || res.clientErr != nil {
				exp := cf.EXPECT().ShardManagerClient(gomock.Any())
				if res.expectArgs != nil {
					exp = exp.Do(func(cmd *cliv3.Command) { res.expectArgs(t, cmd) })
				}
				exp.Return(res.client, res.clientErr).MaxTimes(1)
			}

			cmd := BuildCommandWithFactory(cf)
			buf := new(bytes.Buffer)
			cmd.Writer = buf
			cmd.ErrWriter = buf

			err := cmd.Run(context.Background(), tt.args)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil. out=%s", tt.wantErr, buf.String())
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error: got %q want substring %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if tt.check != nil {
				tt.check(t, buf.String())
			}
		})
	}
}

func TestFormatMetadata(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   map[string]string
		want string
	}{
		{name: "nil renders dash", in: nil, want: "-"},
		{name: "empty renders dash", in: map[string]string{}, want: "-"},
		{name: "single key", in: map[string]string{"zone": "dca1"}, want: "zone=dca1"},
		{
			name: "multiple keys are sorted",
			in:   map[string]string{"zone": "phx1", "ip": "10.0.0.1", "build": "v2"},
			want: "build=v2,ip=10.0.0.1,zone=phx1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatMetadata(tt.in); got != tt.want {
				t.Errorf("formatMetadata(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestShortExecutorStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   types.ExecutorStatus
		want string
	}{
		{in: types.ExecutorStatusINVALID, want: "INVALID"},
		{in: types.ExecutorStatusACTIVE, want: "ACTIVE"},
		{in: types.ExecutorStatusDRAINING, want: "DRAINING"},
		{in: types.ExecutorStatusDRAINED, want: "DRAINED"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := shortExecutorStatus(tt.in); got != tt.want {
				t.Errorf("shortExecutorStatus(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
