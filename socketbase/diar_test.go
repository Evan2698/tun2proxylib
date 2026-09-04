package socketbase

import (
	"net"
	"syscall"
	"testing"
)

func TestHostAddress(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		family int
	}{
		{name: "IPv4", input: "192.0.2.1", family: syscall.AF_INET},
		{name: "IPv6", input: "2001:db8::1", family: syscall.AF_INET6},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ip := net.ParseIP(test.input)
			sa, err := netAddrToSockaddr(ip, 25)
			if err != nil {
				t.Fatalf("netAddrToSockaddr() error = %v", err)
			}
			if got := socketFamily(ip); got != test.family {
				t.Fatalf("socketFamily() = %d, want %d", got, test.family)
			}

			switch test.family {
			case syscall.AF_INET:
				if _, ok := sa.(*syscall.SockaddrInet4); !ok {
					t.Fatalf("sockaddr type = %T, want *syscall.SockaddrInet4", sa)
				}
			case syscall.AF_INET6:
				if _, ok := sa.(*syscall.SockaddrInet6); !ok {
					t.Fatalf("sockaddr type = %T, want *syscall.SockaddrInet6", sa)
				}
			}
		})
	}
}
