package smctl

import (
	"context"
	"strings"
	"testing"

	cliv3 "github.com/urfave/cli/v3"
	"go.uber.org/yarpc/api/transport"

	"github.com/cadence-workflow/shard-manager/common"
	"github.com/cadence-workflow/shard-manager/common/types"
)

// runRoot drives a fresh root command with the given args so flag parsing
// populates cmd.String / cmd.Duration the same way it does in production. It
// returns the *cliv3.Command observed inside the action, plus any Run error.
func runRoot(t *testing.T, args []string, action cliv3.ActionFunc) error {
	t.Helper()
	root := &cliv3.Command{
		Name:   "smctl",
		Flags:  rootFlags(),
		Action: action,
	}
	return root.Run(context.Background(), args)
}

func TestNewClientDispatcher_defaultsAndOverrides(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "no flags uses default 127.0.0.1:7943",
			args: []string{"smctl"},
		},
		{
			name: "--address overrides default host:port",
			args: []string{"smctl", "--" + FlagAddress, "host.example.com:7943"},
		},
		{
			name: "explicit --transport=grpc accepted",
			args: []string{"smctl", "--" + FlagTransport, "grpc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runRoot(t, tt.args, func(_ context.Context, cmd *cliv3.Command) error {
				disp, err := newClientDispatcher(cmd)
				if err != nil {
					return err
				}
				if err := disp.Stop(); err != nil {
					t.Fatalf("Stop: %v", err)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
		})
	}
}

func TestNewClientDispatcher_rejectsUnknownTransport(t *testing.T) {
	err := runRoot(t,
		[]string{"smctl", "--" + FlagTransport, "tchannel"},
		func(_ context.Context, cmd *cliv3.Command) error {
			_, err := newClientDispatcher(cmd)
			return err
		},
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported --"+FlagTransport) {
		t.Fatalf("expected unsupported-transport error, got: %v", err)
	}
}

func TestNewClientDispatcher_rejectsMissingTLSCert(t *testing.T) {
	err := runRoot(t,
		[]string{"smctl", "--" + FlagTLSCertPath, "/path/that/does/not/exist.pem"},
		func(_ context.Context, cmd *cliv3.Command) error {
			_, err := newClientDispatcher(cmd)
			return err
		},
	)
	if err == nil || !strings.Contains(err.Error(), "unable to read server CA certificate") {
		t.Fatalf("expected read-cert error, got: %v", err)
	}
}

func TestVersionMiddleware_injectsCallerHeaders(t *testing.T) {
	mw := &versionMiddleware{}
	req := &transport.Request{
		Caller:    smctlClientName,
		Service:   shardManagerServiceName,
		Procedure: "test",
	}
	stub := &capturingOutbound{}

	if _, err := mw.Call(context.Background(), req, stub); err != nil {
		t.Fatalf("Call: %v", err)
	}

	wantHeaders := map[string]string{
		common.CallerTypeHeaderName: types.CallerTypeCLI.String(),
	}
	for k, v := range wantHeaders {
		if got, ok := stub.req.Headers.Get(k); !ok || got != v {
			t.Errorf("header %q: got %q (ok=%v), want %q", k, got, ok, v)
		}
	}
}

// capturingOutbound is a tiny transport.UnaryOutbound stub that records the
// request handed to it and returns an empty response. It avoids pulling in a
// peer list / dispatcher just to verify that middleware copied headers.
type capturingOutbound struct {
	req *transport.Request
	transport.UnaryOutbound
}

func (c *capturingOutbound) Call(_ context.Context, req *transport.Request) (*transport.Response, error) {
	c.req = req
	return &transport.Response{}, nil
}
