package socks

import (
	"io"
	"net"
	"strconv"
	"sync"
	"tun2proxylib/lwipcore/core"

	"golang.org/x/net/proxy"
)

type tcpHandler struct {
	sync.Mutex

	proxyHost string
	proxyPort uint16
}

// NewTCPHandler ...
func NewTCPHandler(proxyHost string, proxyPort uint16) core.TCPConnHandler {
	return &tcpHandler{
		proxyHost: proxyHost,
		proxyPort: proxyPort,
	}
}

func (h *tcpHandler) Handle(conn net.Conn, target *net.TCPAddr) error {
	dialer, err := proxy.SOCKS5("tcp", core.ParseTCPAddr(h.proxyHost, h.proxyPort).String(), nil, nil)
	if err != nil {
		conn.Close()
		return err
	}

	// Replace with a domain name if target address IP is a fake IP.
	targetHost := target.IP.String()

	dest := net.JoinHostPort(targetHost, strconv.Itoa(target.Port))

	c, err := dialer.Dial(target.Network(), dest)
	if err != nil {
		conn.Close()
		return err
	}

	go h.pipe(c, conn)

	return nil
}

func (h *tcpHandler) pipe(dst net.Conn, src net.Conn) {
	defer dst.Close()
	defer src.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		io.Copy(dst, src)
		wg.Done()
	}()
	go func() {
		io.Copy(src, dst)
		wg.Done()
	}()
	wg.Wait()
}
