package gvisorcore

import (
	glog "gvisor.dev/gvisor/pkg/log"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"gvisor.dev/gvisor/pkg/waiter"
)

func withUDPHandler(handle func(UDPConn)) Option {
	return func(s *stack.Stack) error {
		udpForwarder := udp.NewForwarder(s, func(r *udp.ForwarderRequest) bool {
			var (
				wq waiter.Queue
				id = r.ID()
			)
			ep, err := r.CreateEndpoint(&wq)
			if err != nil {
				glog.Debugf("forward udp request: %s:%d->%s:%d: %s",
					id.RemoteAddress, id.RemotePort, id.LocalAddress, id.LocalPort, err)
				return false
			}

			conn := &udpConn{
				UDPConn: gonet.NewUDPConn(&wq, ep),
				id:      id,
			}
			handle(conn)
			return true
		})
		s.SetTransportProtocolHandler(udp.ProtocolNumber, udpForwarder.HandlePacket)
		return nil
	}
}

type udpConn struct {
	*gonet.UDPConn
	id stack.TransportEndpointID
}

func (c *udpConn) ID() *stack.TransportEndpointID {
	return &c.id
}
