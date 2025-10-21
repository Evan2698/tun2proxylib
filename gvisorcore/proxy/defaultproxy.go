package proxy

import (
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
	"tun2proxylib/gvisorcore"
	"tun2proxylib/gvisorcore/buffer"
	"tun2proxylib/udppackage"

	"golang.org/x/net/proxy"
)

var timeout = 30 * time.Second

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
	id := conn.ID()
	srcIP := id.RemoteAddress
	srcPort := id.RemotePort
	dstIP := id.LocalAddress
	dstPort := id.LocalPort

	log.Printf("TCP stream--->srcIP: %v, srcPort: %v, dstIP: %v, dstPort: %v", srcIP, srcPort, dstIP, dstPort)

	remoteAddress := net.JoinHostPort(dstIP.String(), strconv.Itoa(int(dstPort)))

	proxyConn, err := dialer.Dial("tcp", remoteAddress)
	if err != nil {
		conn.Close()
		return
	}
	go func() {
		defer conn.Close()
		defer proxyConn.Close()
		//gvisorcore.Relay(conn, proxyConn)
		log.Println("relay of conn")
		var wg sync.WaitGroup
		wg.Add(2)
		go copySource2Destination(conn, proxyConn, &wg)
		go copySource2Destination(proxyConn, conn, &wg)
		wg.Wait()
	}()

}

func copySource2Destination(s, d net.Conn, w *sync.WaitGroup) {
	s.SetReadDeadline(time.Now().Add(timeout))
	d.SetWriteDeadline(time.Now().Add(timeout))
	io.Copy(s, d)
	w.Done()
}

// HandleUDP handles UDP packets by forwarding them to the specified UDP proxy server.
// It reads packets from the gVisor UDP connection, encapsulates them, and sends them to the proxy server.
// It also listens for responses from the proxy server and forwards them back to the original sender.

func (p *DefaultProxy) HandleUDP(conn gvisorcore.UDPConn) {
	id := conn.ID()
	srcIP := id.RemoteAddress
	srcPort := id.RemotePort
	dstIP := id.LocalAddress
	dstPort := id.LocalPort

	log.Printf("UDP packet: srcIP: %v, srcPort: %v, dstIP: %v, dstPort: %v", srcIP, srcPort, dstIP, dstPort)

	dest := net.JoinHostPort(dstIP.String(), strconv.Itoa(int(dstPort)))
	src := net.JoinHostPort(srcIP.String(), strconv.Itoa(int(srcPort)))
	destAddr, err := parseAddress(dest)
	if err != nil {
		conn.Close()
		return
	}
	srcAddr, err := parseAddress(src)
	if err != nil {
		conn.Close()
		return
	}

	addr, err := net.ResolveUDPAddr("udp", p.UDPUrl)
	if err != nil {
		conn.Close()
		return
	}

	rawConn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		conn.Close()
		return
	}

	go func() {
		defer conn.Close()
		defer rawConn.Close()
		var wg sync.WaitGroup
		wg.Add(2)

		go sendUdpPacket2RemoteDestination(conn, destAddr, srcAddr, rawConn, &wg)
		go copyFromRemote2LocalDestination(rawConn, conn, &wg, &srcAddr)

		wg.Wait()
	}()

}

func copyFromRemote2LocalDestination(rawConn *net.UDPConn, conn gvisorcore.UDPConn, wg *sync.WaitGroup, to net.Addr) {
	buf := buffer.Get()
	defer buffer.Put(buf)

	for {
		rawConn.SetReadDeadline(time.Now().Add(timeout))
		n, _, err := rawConn.ReadFrom(buf[:buffer.TriplePage])
		if err != nil {
			break
		}
		if n == 0 {
			continue
		}
		_, _, payload, err := udppackage.UnpackUDPData(buf[:n])
		if err != nil {
			break
		}
		conn.SetWriteDeadline(time.Now().Add(timeout))
		_, err = conn.WriteTo(payload, to)
		if err != nil {
			break
		}
	}

	wg.Done()
}

func sendUdpPacket2RemoteDestination(conn gvisorcore.UDPConn, destAddr net.UDPAddr, srcAddr net.UDPAddr, rawConn *net.UDPConn, wg *sync.WaitGroup) {
	buf := buffer.Get()
	defer buffer.Put(buf)

	for {
		conn.SetReadDeadline(time.Now().Add(timeout))
		n, _, err := conn.ReadFrom(buf[:buffer.TriplePage])
		if err != nil {
			break
		}
		packedData, err := udppackage.PackUDPData(&destAddr, &srcAddr, buf[:n])
		if err != nil {
			continue
		}
		rawConn.SetWriteDeadline(time.Now().Add(timeout))
		_, err = rawConn.Write(packedData)
		if err != nil {
			break
		}
	}

	wg.Done()
}

func parseAddress(address string) (net.UDPAddr, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return net.UDPAddr{}, err
	}
	return *udpAddr, nil
}
