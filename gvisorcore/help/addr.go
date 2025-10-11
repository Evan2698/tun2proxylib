package help

import (
	"net"
	"net/netip"

	"gvisor.dev/gvisor/pkg/tcpip"
)

func ParseNetAddr(addr net.Addr) (netip.Addr, uint16) {
	if addr == nil {
		return netip.Addr{}, 0
	}
	if v, ok := addr.(interface {
		AddrPort() netip.AddrPort
	}); ok {
		ap := v.AddrPort()
		return ap.Addr(), ap.Port()
	}
	return ParseAddrString(addr.String())
}

// parseAddrString parses address string to IP and port.
// It doesn't do any name resolution.
func ParseAddrString(s string) (netip.Addr, uint16) {
	ap, err := netip.ParseAddrPort(s)
	if err != nil {
		return netip.Addr{}, 0
	}
	return ap.Addr(), ap.Port()
}

// parseTCPIPAddress parses tcpip.Address to netip.Addr.
func ParseTCPIPAddress(addr tcpip.Address) netip.Addr {
	ip, _ := netip.AddrFromSlice(addr.AsSlice())
	return ip
}
