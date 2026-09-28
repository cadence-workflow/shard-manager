package smctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
				HostMetadata:   &types.HostMetadata{HostName: "host-1"},
				AssignedShards: []*types.ExecutorAssignedShardState{{ShardKey: "shard-3"}},
			},
			{
				ExecutorID:     "executor-a",
				HostMetadata:   &types.HostMetadata{HostName: "host-1"},
				AssignedShards: []*types.ExecutorAssignedShardState{{ShardKey: "shard-1"}, {ShardKey: "shard-2"}},
			},
			{
				ExecutorID:     "executor-c",
				HostMetadata:   &types.HostMetadata{HostName: "host-2"},
				AssignedShards: []*types.ExecutorAssignedShardState{{ShardKey: "shard-4"}},
			},
			{
				ExecutorID:     "executor-no-host",
				AssignedShards: []*types.ExecutorAssignedShardState{{ShardKey: "shard-5"}},
			},
		},
	}
	drainArgs := []string{
		"smctl", "-n", "ns-1", "executor", "drain",
		"--" + FlagHostname, "host-1",
		"--" + FlagHostname, "host-3",
		"--" + FlagReason, "maintenance",
		"--" + FlagDrainedBy, "operator",
	}
	prompt := "Host \"host-1\" currently holds 3 shard(s):\n" +
		"  executor executor-a: 2 shard(s)\n" +
		"  executor executor-b: 1 shard(s)\n" +
		"Host \"host-3\" has no executors registered in this namespace.\n" +
		"Proceed with draining? [y/N]: "
	drainReq := &types.DrainHostsRequest{
		Namespace: "ns-1",
		Hosts: []*types.DrainedHost{
			{Hostname: "host-1", DrainedBy: "operator", Reason: "maintenance"},
			{Hostname: "host-3", DrainedBy: "operator", Reason: "maintenance"},
		},
	}
	expectState := func(mc *sharddistributor.MockClient) {
		mc.EXPECT().
			GetNamespaceState(gomock.Any(), &types.GetNamespaceStateRequest{Namespace: "ns-1"}).
			Return(namespaceState, nil)
	}

	runHostDrainCommandTests(t, []hostDrainCommandTest{
		{
			name:  "confirmed: prints shard counts then drains",
			args:  drainArgs,
			stdin: "YES\n",
			setup: func(mc *sharddistributor.MockClient) sharddistributor.Client {
				expectState(mc)
				mc.EXPECT().DrainHosts(gomock.Any(), drainReq).Return(nil)
				return mc
			},
			wantPrompt: prompt,
			check: func(t *testing.T, stdout string) {
				var got types.DrainHostsRequest
				if err := json.Unmarshal([]byte(stdout), &got); err != nil {
					t.Fatalf("output is not valid JSON: %v\nout: %s", err, stdout)
				}
				if len(got.Hosts) != 2 || got.Hosts[0].Hostname != "host-1" || got.Hosts[1].Hostname != "host-3" {
					t.Errorf("unexpected hosts in output: %s", stdout)
				}
			},
		},
		{
			name:  "declined: does not drain",
			args:  drainArgs,
			stdin: "n\n",
			setup: func(mc *sharddistributor.MockClient) sharddistributor.Client {
				expectState(mc)
				return mc
			},
			wantPrompt: prompt,
			wantErr:    "drain aborted",
		},
		{
			name: "no input: does not drain",
			args: drainArgs,
			setup: func(mc *sharddistributor.MockClient) sharddistributor.Client {
				expectState(mc)
				return mc
			},
			wantPrompt: prompt,
			wantErr:    "drain aborted: no input received",
		},
		{
			name: "--yes skips the prompt",
			args: []string{"smctl", "-n", "ns-1", "executor", "drain", "--" + FlagHostname, "host-2", "--" + FlagDrainedBy, "operator", "-y"},
			setup: func(mc *sharddistributor.MockClient) sharddistributor.Client {
				mc.EXPECT().
					DrainHosts(gomock.Any(), &types.DrainHostsRequest{
						Namespace: "ns-1",
						Hosts:     []*types.DrainedHost{{Hostname: "host-2", DrainedBy: "operator"}},
					}).
					Return(nil)
				return mc
			},
		},
		{
			name: "GetNamespaceState error is propagated",
			args: drainArgs,
			setup: func(mc *sharddistributor.MockClient) sharddistributor.Client {
				mc.EXPECT().
					GetNamespaceState(gomock.Any(), gomock.Any()).
					Return(nil, &types.NamespaceNotFoundError{Namespace: "ns-1"})
				return mc
			},
			wantErr: `GetNamespaceState: namespace "ns-1" not found`,
		},
		{
			name:  "DrainHosts error is propagated",
			args:  drainArgs,
			stdin: "y\n",
			setup: func(mc *sharddistributor.MockClient) sharddistributor.Client {
				expectState(mc)
				mc.EXPECT().DrainHosts(gomock.Any(), drainReq).Return(&types.BadRequestError{Message: "invalid hostname"})
				return mc
			},
			wantPrompt: prompt,
			wantErr:    "DrainHosts: invalid hostname",
		},
		{
			name:    "missing --hostname fails",
			args:    []string{"smctl", "-n", "ns-1", "executor", "drain"},
			wantErr: `Required flag "` + FlagHostname + `" not set`,
		},
	})
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
		{
			name:    "missing --namespace fails",
			args:    []string{"smctl", "executor", "undrain", "--" + FlagHostname, "host-1"},
			wantErr: `--` + FlagNamespace + ` is required`,
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
