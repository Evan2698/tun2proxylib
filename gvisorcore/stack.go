//go:build !windows
// +build !windows

package gvisorcore

import (
	"gvisor.dev/gvisor/pkg/tcpip/link/fdbased"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/icmp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
)

type StackOptions struct {
	// TransportHandler handles transport layer packets (TCP/UDP).
	TransportHandler TransportHandler

	// LinkEndpoint is the link endpoint to be attached to the stack NIC.
	LinkEndpoint stack.LinkEndpoint
}

func CreateStack(cfg StackOptions) (*stack.Stack, error) {

	opts := []Option{WithDefault()}

	s := stack.New(stack.Options{
		NetworkProtocols: []stack.NetworkProtocolFactory{
			ipv4.NewProtocol,
			ipv6.NewProtocol,
		},
		TransportProtocols: []stack.TransportProtocolFactory{
			tcp.NewProtocol,
			udp.NewProtocol,
			icmp.NewProtocol4,
			icmp.NewProtocol6,
		},
	})

	// Generate unique NIC id.
	nicID := s.NextNICID()

	opts = append(opts,
		// Important: We must initiate transport protocol handlers
		// before creating NIC, otherwise NIC would dispatch packets
		// to stack and cause race condition.
		// Initiate transport protocol (TCP/UDP) with given handler.
		withTCPHandler(cfg.TransportHandler.HandleTCP),
		withUDPHandler(cfg.TransportHandler.HandleUDP),

		// Create stack NIC and then bind link endpoint to it.
		withCreatingNIC(nicID, cfg.LinkEndpoint),

		// In the past we did s.AddAddressRange to assign 0.0.0.0/0
		// onto the interface. We need that to be able to terminate
		// all the incoming connections - to any ip. AddressRange API
		// has been removed and the suggested workaround is to use
		// Promiscuous mode. https://github.com/google/gvisor/issues/3876
		//
		// Ref: https://github.com/cloudflare/slirpnetstack/blob/master/stack.go
		withPromiscuousMode(nicID, nicPromiscuousModeEnabled),

		// Enable spoofing if a stack may send packets from unowned
		// addresses. This change required changes to some netgophers
		// since previously, promiscuous mode was enough to let the
		// netstack respond to all incoming packets regardless of the
		// packet's destination address. Now that a stack.Route is not
		// held for each incoming packet, finding a route may fail with
		// local addresses we don't own but accepted packets for while
		// in promiscuous mode. Since we also want to be able to send
		// from any address (in response the received promiscuous mode
		// packets), we need to enable spoofing.
		//
		// Ref: https://github.com/google/gvisor/commit/8c0701462a84ff77e602f1626aec49479c308127
		withSpoofing(nicID, nicSpoofingEnabled),

		// Add default route table for IPv4 and IPv6. This will handle
		// all incoming ICMP packets.
		withRouteTable(nicID),

		// Add default NIC to the given multicast groups.
		//withMulticastGroups(nicID, MulticastGroups),
	)

	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func CreateLinkEndpoint(fd int, mtu uint32) (stack.LinkEndpoint, error) {
	// 使用 gvisor 的 tun/tap endpoint 创建 LinkEndpoint
	// 需要导入 "gvisor.dev/gvisor/pkg/tcpip/link/tun"
	linkEP, err := fdbased.New(&fdbased.Options{
		FDs: []int{fd},
		MTU: mtu,
	})

	return linkEP, err

	//return nil, nil
}
