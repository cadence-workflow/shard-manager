package smctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/client/sharddistributor"
	"github.com/cadence-workflow/shard-manager/common/types"
)

type hostDrainCommandTest struct {
	name  string
	args  []string
	stdin string
	// setup returns the client handed out by the factory; nil means the
	// command must fail before asking for one.
	setup      func(mc *sharddistributor.MockClient) sharddistributor.Client
	wantErr    string
	wantPrompt string
	check      func(t *testing.T, stdout string)
}

func runHostDrainCommandTests(t *testing.T, tests []hostDrainCommandTest) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			cf := NewMockClientFactory(ctrl)
			cf.EXPECT().Close().Return(nil)
			if tt.setup != nil {
				cf.EXPECT().ShardManagerClient(gomock.Any()).Return(tt.setup(sharddistributor.NewMockClient(ctrl)), nil)
			}

			cmd := BuildCommandWithFactory(cf)
			buf := new(bytes.Buffer)
			cmd.Writer = buf
			cmd.ErrWriter = buf
			cmd.Reader = strings.NewReader(tt.stdin)

			err := cmd.Run(context.Background(), tt.args)

			stdout := buf.String()
			if !strings.HasPrefix(stdout, tt.wantPrompt) {
				t.Fatalf("prompt mismatch:\ngot:\n%s\nwant prefix:\n%s", stdout, tt.wantPrompt)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error: got %v want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if tt.check != nil {
				tt.check(t, strings.TrimPrefix(stdout, tt.wantPrompt))
			}
		})
	}
}

func TestDrainHosts(t *testing.T) {
	namespaceState := &types.GetNamespaceStateResponse{
		Namespace: "ns-1",
		Executors: []*types.NamespaceExecutorState{
			{
				ExecutorID:     "executor-b",
				Status:         types.ExecutorStatusACTIVE,
				HostMetadata:   &types.HostMetadata{HostName: "host-1"},
				AssignedShards: []*types.ExecutorAssignedShardState{{ShardKey: "shard-3"}},
			},
			{
				ExecutorID:     "executor-a",
				Status:         types.ExecutorStatusACTIVE,
				HostMetadata:   &types.HostMetadata{HostName: "host-1"},
				AssignedShards: []*types.ExecutorAssignedShardState{{ShardKey: "shard-1"}, {ShardKey: "shard-2"}},
			},
			{
				ExecutorID:     "executor-c",
				Status:         types.ExecutorStatusACTIVE,
				HostMetadata:   &types.HostMetadata{HostName: "host-2"},
				AssignedShards: []*types.ExecutorAssignedShardState{{ShardKey: "shard-4"}},
			},
			{
				ExecutorID:     "executor-no-host",
				Status:         types.ExecutorStatusACTIVE,
				AssignedShards: []*types.ExecutorAssignedShardState{{ShardKey: "shard-5"}},
			},
		},
	}
	drainArgs := []string{
		"smctl", "-n", "ns-1", "executor", "drain",
		"--" + FlagHostname, "host-1",
		"--" + FlagHostname, "host-3",
		"--" + FlagReason, "maintenance",
	}
	prompt := "Host \"host-1\" currently holds 3 shard(s):\n" +
		"  executor executor-a: 2 shard(s)\n" +
		"  executor executor-b: 1 shard(s)\n" +
		"Hostname \"host-3\" is not known in this namespace — it has no registered executors. " +
		"If this is unexpected, check for a typo before proceeding.\n" +
		"Proceed with draining? [y/N]: "
	expectConfirmRPCs := func(mc *sharddistributor.MockClient) {
		mc.EXPECT().
			GetNamespaceState(gomock.Any(), &types.GetNamespaceStateRequest{Namespace: "ns-1"}).
			Return(namespaceState, nil)
		mc.EXPECT().
			GetDrainedHosts(gomock.Any(), &types.GetDrainedHostsRequest{Namespace: "ns-1"}).
			Return(&types.GetDrainedHostsResponse{Namespace: "ns-1"}, nil)
	}
	expectDrainHosts := func(mc *sharddistributor.MockClient, hostnames []string, reason string) {
		mc.EXPECT().DrainHosts(gomock.Any(), gomock.AssignableToTypeOf(&types.DrainHostsRequest{})).DoAndReturn(
			func(_ context.Context, req *types.DrainHostsRequest, _ ...any) error {
				if req.GetNamespace() != "ns-1" {
					return fmt.Errorf("unexpected namespace %q", req.GetNamespace())
				}
				if len(req.GetHosts()) != len(hostnames) {
					return fmt.Errorf("unexpected host count %d", len(req.GetHosts()))
				}
				for i, hostname := range hostnames {
					host := req.Hosts[i]
					if host.Hostname != hostname {
						return fmt.Errorf("host[%d]: got %q want %q", i, host.Hostname, hostname)
					}
					if host.Reason != reason {
						return fmt.Errorf("host[%d] reason: got %q want %q", i, host.Reason, reason)
					}
					if host.DrainedBy == "" {
						return fmt.Errorf("host[%d]: DrainedBy should be set from the local username", i)
					}
				}
				return nil
			},
		)
	}

	runHostDrainCommandTests(t, []hostDrainCommandTest{
		{
			name:  "confirmed: prints shard counts then drains",
			args:  drainArgs,
			stdin: "YES\n",
			setup: func(mc *sharddistributor.MockClient) sharddistributor.Client {
				expectConfirmRPCs(mc)
				expectDrainHosts(mc, []string{"host-1", "host-3"}, "maintenance")
				return mc
			},
			wantPrompt: prompt,
			check: func(t *testing.T, stdout string) {
				if !strings.HasPrefix(stdout, drainTakesEffectNotice+"\n") {
					t.Fatalf("missing rebalance notice:\n%s", stdout)
				}
				jsonOut := strings.TrimPrefix(stdout, drainTakesEffectNotice+"\n")
				var got drainHostsEcho
				if err := json.Unmarshal([]byte(jsonOut), &got); err != nil {
					t.Fatalf("output is not valid JSON: %v\nout: %s", err, jsonOut)
				}
				if got.Namespace != "ns-1" || len(got.Hosts) != 2 {
					t.Errorf("unexpected echo: %s", jsonOut)
				}
				if got.Hosts[0].Hostname != "host-1" || got.Hosts[1].Hostname != "host-3" {
					t.Errorf("unexpected hosts in output: %s", jsonOut)
				}
				if strings.Contains(jsonOut, "DrainedAt") || strings.Contains(jsonOut, "0001-01-01") {
					t.Errorf("output should omit DrainedAt: %s", jsonOut)
				}
			},
		},
		{
			name:  "declined: does not drain",
			args:  drainArgs,
			stdin: "n\n",
			setup: func(mc *sharddistributor.MockClient) sharddistributor.Client {
				expectConfirmRPCs(mc)
				return mc
			},
			wantPrompt: prompt,
			wantErr:    "drain aborted",
		},
		{
			name:  "warns when drain would leave no assignable executors",
			args:  []string{"smctl", "-n", "ns-1", "executor", "drain", "--" + FlagHostname, "host-1"},
			stdin: "y\n",
			setup: func(mc *sharddistributor.MockClient) sharddistributor.Client {
				mc.EXPECT().
					GetNamespaceState(gomock.Any(), &types.GetNamespaceStateRequest{Namespace: "ns-1"}).
					Return(&types.GetNamespaceStateResponse{
						Namespace: "ns-1",
						Executors: []*types.NamespaceExecutorState{
							{
								ExecutorID:     "executor-a",
								Status:         types.ExecutorStatusACTIVE,
								HostMetadata:   &types.HostMetadata{HostName: "host-1"},
								AssignedShards: []*types.ExecutorAssignedShardState{{ShardKey: "shard-1"}},
							},
						},
					}, nil)
				mc.EXPECT().
					GetDrainedHosts(gomock.Any(), &types.GetDrainedHostsRequest{Namespace: "ns-1"}).
					Return(&types.GetDrainedHostsResponse{Namespace: "ns-1"}, nil)
				expectDrainHosts(mc, []string{"host-1"}, "")
				return mc
			},
			wantPrompt: "Host \"host-1\" currently holds 1 shard(s):\n" +
				"  executor executor-a: 1 shard(s)\n" +
				lastAssignableExecutorsWarning + "\n" +
				"Proceed with draining? [y/N]: ",
			check: func(t *testing.T, stdout string) {
				if !strings.Contains(stdout, drainTakesEffectNotice) {
					t.Fatalf("missing rebalance notice:\n%s", stdout)
				}
			},
		},
		{
			name:  "DrainHosts error is propagated",
			args:  drainArgs,
			stdin: "y\n",
			setup: func(mc *sharddistributor.MockClient) sharddistributor.Client {
				expectConfirmRPCs(mc)
				mc.EXPECT().DrainHosts(gomock.Any(), gomock.Any()).
					Return(&types.BadRequestError{Message: "invalid hostname"})
				return mc
			},
			wantPrompt: prompt,
			wantErr:    "DrainHosts: invalid hostname",
		},
	})
}

func TestWouldLeaveNoAssignableExecutors(t *testing.T) {
	active := func(id, hostname string) *types.NamespaceExecutorState {
		ex := &types.NamespaceExecutorState{
			ExecutorID: id,
			Status:     types.ExecutorStatusACTIVE,
		}
		if hostname != "" {
			ex.HostMetadata = &types.HostMetadata{HostName: hostname}
		}
		return ex
	}

	tests := []struct {
		name           string
		hostnames      []string
		executors      []*types.NamespaceExecutorState
		alreadyDrained map[string]struct{}
		want           bool
	}{
		{
			name:      "other active host remains",
			hostnames: []string{"host-1"},
			executors: []*types.NamespaceExecutorState{active("a", "host-1"), active("b", "host-2")},
			want:      false,
		},
		{
			name:      "draining the only active host",
			hostnames: []string{"host-1"},
			executors: []*types.NamespaceExecutorState{active("a", "host-1")},
			want:      true,
		},
		{
			name:           "remaining host already drained",
			hostnames:      []string{"host-1"},
			executors:      []*types.NamespaceExecutorState{active("a", "host-1"), active("b", "host-2")},
			alreadyDrained: map[string]struct{}{"host-2": {}},
			want:           true,
		},
		{
			name:      "executor without hostname remains assignable",
			hostnames: []string{"host-1"},
			executors: []*types.NamespaceExecutorState{active("a", "host-1"), active("b", "")},
			want:      false,
		},
		{
			name:      "non-active executors do not count",
			hostnames: []string{"host-1"},
			executors: []*types.NamespaceExecutorState{
				active("a", "host-1"),
				{ExecutorID: "b", Status: types.ExecutorStatusDRAINING, HostMetadata: &types.HostMetadata{HostName: "host-2"}},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wouldLeaveNoAssignableExecutors(tt.hostnames, tt.executors, tt.alreadyDrained)
			if got != tt.want {
				t.Errorf("wouldLeaveNoAssignableExecutors(...) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUndrainHosts(t *testing.T) {
	runHostDrainCommandTests(t, []hostDrainCommandTest{
		{
			name: "success: prints undrained hostnames",
			args: []string{"smctl", "-n", "ns-1", "executor", "undrain", "--" + FlagHostname, "host-1", "--" + FlagHostname, "host-2"},
			setup: func(mc *sharddistributor.MockClient) sharddistributor.Client {
				mc.EXPECT().
					UndrainHosts(gomock.Any(), &types.UndrainHostsRequest{
						Namespace: "ns-1",
						Hostnames: []string{"host-1", "host-2"},
					}).
					Return(&types.UndrainHostsResponse{UndrainedHostnames: []string{"host-1"}}, nil)
				return mc
			},
			check: func(t *testing.T, stdout string) {
				var got types.UndrainHostsResponse
				if err := json.Unmarshal([]byte(stdout), &got); err != nil {
					t.Fatalf("output is not valid JSON: %v\nout: %s", err, stdout)
				}
				if strings.Join(got.UndrainedHostnames, ",") != "host-1" {
					t.Errorf("UndrainedHostnames: got %v want [host-1]", got.UndrainedHostnames)
				}
			},
		},
		{
			name: "API error is propagated",
			args: []string{"smctl", "-n", "ns-1", "executor", "undrain", "--" + FlagHostname, "host-1"},
			setup: func(mc *sharddistributor.MockClient) sharddistributor.Client {
				mc.EXPECT().UndrainHosts(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))
				return mc
			},
			wantErr: "UndrainHosts: boom",
		},
	})
}

func TestGetDrainedHosts(t *testing.T) {
	drainedAt := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	runHostDrainCommandTests(t, []hostDrainCommandTest{
		{
			name: "success: prints drained hosts with metadata",
			args: []string{"smctl", "-n", "ns-1", "executor", "list-drained"},
			setup: func(mc *sharddistributor.MockClient) sharddistributor.Client {
				mc.EXPECT().
					GetDrainedHosts(gomock.Any(), &types.GetDrainedHostsRequest{Namespace: "ns-1"}).
					Return(&types.GetDrainedHostsResponse{
						Namespace: "ns-1",
						Hosts: []*types.DrainedHost{
							{Hostname: "host-1", DrainedAt: drainedAt, DrainedBy: "operator", Reason: "maintenance"},
						},
					}, nil)
				return mc
			},
			check: func(t *testing.T, stdout string) {
				var got types.GetDrainedHostsResponse
				if err := json.Unmarshal([]byte(stdout), &got); err != nil {
					t.Fatalf("output is not valid JSON: %v\nout: %s", err, stdout)
				}
				want := types.DrainedHost{Hostname: "host-1", DrainedAt: drainedAt, DrainedBy: "operator", Reason: "maintenance"}
				if len(got.Hosts) != 1 || *got.Hosts[0] != want {
					t.Errorf("Hosts: got %s", stdout)
				}
			},
		},
		{
			name: "API error is propagated",
			args: []string{"smctl", "-n", "ns-1", "executor", "list-drained"},
			setup: func(mc *sharddistributor.MockClient) sharddistributor.Client {
				mc.EXPECT().GetDrainedHosts(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))
				return mc
			},
			wantErr: "GetDrainedHosts: boom",
		},
	})
}
