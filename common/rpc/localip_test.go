package rpc

// ** This code is copied from tchannel, we would like to not take dependency on tchannel code **

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cadence-workflow/shard-manager/common/config"
)

func TestScoreAddr(t *testing.T) {
	ipv4 := net.ParseIP("10.0.1.2")
	ipv6 := net.ParseIP("2001:db8:a0b:12f0::1")

	tests := []struct {
		msg    string
		iface  net.Interface
		addr   net.Addr
		want   int
		wantIP net.IP
	}{
		{
			msg:    "non-local up ipv4 IPNet address",
			iface:  net.Interface{Flags: net.FlagUp},
			addr:   &net.IPNet{IP: ipv4},
			want:   500,
			wantIP: ipv4,
		},
		{
			msg:    "non-local up ipv4 IPAddr address",
			iface:  net.Interface{Flags: net.FlagUp},
			addr:   &net.IPAddr{IP: ipv4},
			want:   500,
			wantIP: ipv4,
		},
		{
			msg: "non-local up ipv4 IPAddr address, docker interface",
			iface: net.Interface{
				Flags:        net.FlagUp,
				HardwareAddr: mustParseMAC("02:42:ac:11:56:af"),
			},
			addr:   &net.IPNet{IP: ipv4},
			want:   500,
			wantIP: ipv4,
		},
		{
			msg: "non-local up ipv4 address, local MAC address",
			iface: net.Interface{
				Flags:        net.FlagUp,
				HardwareAddr: mustParseMAC("02:42:9c:52:fc:86"),
			},
			addr:   &net.IPNet{IP: ipv4},
			want:   500,
			wantIP: ipv4,
		},
		{
			msg:    "non-local down ipv4 address",
			iface:  net.Interface{},
			addr:   &net.IPNet{IP: ipv4},
			want:   400,
			wantIP: ipv4,
		},
		{
			msg:    "non-local down ipv6 address",
			iface:  net.Interface{},
			addr:   &net.IPAddr{IP: ipv6},
			want:   100,
			wantIP: ipv6,
		},
		{
			msg:   "unknown address type",
			iface: net.Interface{},
			addr:  &net.UnixAddr{Name: "/tmp/socket"},
			want:  -1,
		},
	}

	for _, tt := range tests {
		gotScore, gotIP := scoreAddr(tt.iface, tt.addr)
		assert.Equal(t, tt.want, gotScore, tt.msg)
		assert.Equal(t, tt.wantIP, gotIP, tt.msg)
	}
}

func TestGetListenIP(t *testing.T) {
	tests := []struct {
		msg     string
		cfg     config.RPC
		wantIP  net.IP
		wantErr string
	}{
		{
			msg:    "bind on localhost",
			cfg:    config.RPC{BindOnLocalHost: true},
			wantIP: net.IPv4(127, 0, 0, 1),
		},
		{
			msg:    "bind on specific IPv4",
			cfg:    config.RPC{BindOnIP: "1.2.3.4"},
			wantIP: net.ParseIP("1.2.3.4").To4(),
		},
		{
			msg:     "mutually exclusive options",
			cfg:     config.RPC{BindOnLocalHost: true, BindOnIP: "1.2.3.4"},
			wantErr: "bindOnLocalHost and bindOnIP are mutually exclusive",
		},
		{
			msg:     "invalid IP",
			cfg:     config.RPC{BindOnIP: "invalidIP"},
			wantErr: "unable to parse bindOnIP value or it is not an IPv4 or IPv6 address: invalidIP",
		},
		{
			msg: "auto-detect fallback",
			cfg: config.RPC{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			ip, err := GetListenIP(tt.cfg)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
			if tt.wantIP != nil {
				assert.Equal(t, tt.wantIP, ip)
			} else {
				assert.NotNil(t, ip)
			}
		})
	}
}
