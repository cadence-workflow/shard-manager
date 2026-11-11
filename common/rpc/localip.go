package rpc

// ** This code is copied from tchannel, we would like to not take dependency on tchannel code **

import (
	"errors"
	"fmt"
	"net"

	"github.com/cadence-workflow/shard-manager/common/config"
)

// scoreAddr scores how likely the given addr is to be a remote address and returns the
// IP to use when listening. Any address which receives a negative score should not be used.
// Scores are calculated as:
// -1 for any unknown IP addreseses.
// +300 for IPv4 addresses
// +100 for non-local addresses, extra +100 for "up" interaces.
func scoreAddr(iface net.Interface, addr net.Addr) (int, net.IP) {
	var ip net.IP
	if netAddr, ok := addr.(*net.IPNet); ok {
		ip = netAddr.IP
	} else if netIP, ok := addr.(*net.IPAddr); ok {
		ip = netIP.IP
	} else {
		return -1, nil
	}

	var score int
	if ip.To4() != nil {
		score += 300
	}
	if iface.Flags&net.FlagLoopback == 0 && !ip.IsLoopback() {
		score += 100
		if iface.Flags&net.FlagUp != 0 {
			score += 100
		}
	}
	return score, ip
}

// ListenIP returns the IP to bind to in Listen. It tries to find an IP that can be used
// by other machines to reach this machine.
func ListenIP() (net.IP, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	bestScore := -1
	var bestIP net.IP
	// Select the highest scoring IP as the best IP.
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			// Skip this interface if there is an error.
			continue
		}

		for _, addr := range addrs {
			score, ip := scoreAddr(iface, addr)
			if score > bestScore {
				bestScore = score
				bestIP = ip
			}
		}
	}

	if bestScore == -1 {
		return nil, errors.New("no addresses to listen on")
	}

	return bestIP, nil
}

// GetListenIP returns the IP address to bind/listen on based on RPC config.
// It respects bindOnLocalHost, bindOnIP, or falls back to auto-detection.
func GetListenIP(cfg config.RPC) (net.IP, error) {
	if cfg.BindOnLocalHost && len(cfg.BindOnIP) > 0 {
		return nil, fmt.Errorf("bindOnLocalHost and bindOnIP are mutually exclusive")
	}

	if cfg.BindOnLocalHost {
		return net.IPv4(127, 0, 0, 1), nil
	}

	if len(cfg.BindOnIP) > 0 {
		ip := net.ParseIP(cfg.BindOnIP)
		if ip != nil && ip.To4() != nil {
			return ip.To4(), nil
		}
		if ip != nil && ip.To16() != nil {
			return ip.To16(), nil
		}
		return nil, fmt.Errorf("unable to parse bindOnIP value or it is not an IPv4 or IPv6 address: %s", cfg.BindOnIP)
	}
	return ListenIP()
}

func mustParseMAC(s string) net.HardwareAddr {
	addr, err := net.ParseMAC(s)
	if err != nil {
		panic(err)
	}
	return addr
}
