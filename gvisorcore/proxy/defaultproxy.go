package proxy

import (
	"tun2proxylib/gvisorcore"

	"golang.org/x/net/proxy"
)

type DefaultProxy struct {
	TCPUrl string
	UDPUrl string
}

func NewDefaultProxy(tcpUrl, udpUrl string) *DefaultProxy {
	return &DefaultProxy{
		TCPUrl: tcpUrl,
		UDPUrl: udpUrl,
	}
}

func (p *DefaultProxy) HandleTCP(conn gvisorcore.TCPConn) {
	dialer, err := proxy.SOCKS5("tcp", p.TCPUrl, nil, nil)
	if err != nil {
		conn.Close()
		return
	}
	proxyConn, err := dialer.Dial("tcp", conn.RemoteAddr().String())
	if err != nil {
		conn.Close()
		return
	}
	go func() {
		defer conn.Close()
		defer proxyConn.Close()
		//gvisorcore.Relay(conn, proxyConn)
	}()

}

func (p *DefaultProxy) HandleUDP(conn gvisorcore.UDPConn) {

}
