package socks

import (
	"errors"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
	"tun2proxylib/lwipcore/common/dns"
	"tun2proxylib/lwipcore/common/dns/cache"
	"tun2proxylib/lwipcore/core"
	"tun2proxylib/udppackage"
)

type udpHandler struct {
	sync.Mutex

	proxyHost string
	proxyPort uint16
	udpSocks  map[core.UDPConn]net.Conn
	timeout   time.Duration

	dnsCache *cache.DNSCache
}

const timeoutSecond = 30 * 60 * 12 // 30 minutes

var (
	connectionTimeMap = make(map[net.Conn]int32)
	once              sync.Once
	lock              sync.Mutex
)

// NewUDPHandler ...
func NewUDPHandler(proxyHost string, proxyPort uint16, timeout time.Duration, dnsCache *cache.DNSCache) core.UDPConnHandler {

	once.Do(initTimer)
	return &udpHandler{
		proxyHost: proxyHost,
		proxyPort: proxyPort,
		dnsCache:  dnsCache,
		timeout:   timeout,
		udpSocks:  make(map[core.UDPConn]net.Conn, 8),
	}
}

func initTimer() {

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		for {
			<-ticker.C
			lock.Lock()
			for con, t := range connectionTimeMap {
				t--
				if t == 0 {
					delete(connectionTimeMap, con)
					con.Close()
				}
			}
			lock.Unlock()

			if core.Exit {
				break
			}
		}
	}()
}

func settimeout(con net.Conn, second time.Duration) {
	readTimeout := second
	v := time.Now().Add(readTimeout)
	con.SetReadDeadline(v)
	con.SetWriteDeadline(v)
	con.SetDeadline(v)
}

// Connect ...
func (h *udpHandler) Connect(conn core.UDPConn, target *net.UDPAddr) error {
	dest := net.JoinHostPort(h.proxyHost, strconv.Itoa(int(h.proxyPort)))
	remoteCon, err := net.Dial("udp", dest)
	if err != nil || target == nil {
		log.Println("socks connect failed:", err, dest)
		return err
	}

	lock.Lock()
	connectionTimeMap[remoteCon] = timeoutSecond // 不管是否已经有，都把时间重置
	lock.Unlock()

	h.Lock()
	v, ok := h.udpSocks[conn]
	if ok {
		delete(h.udpSocks, conn)
		v.Close()
	}
	h.udpSocks[conn] = remoteCon
	h.Unlock()

	settimeout(remoteCon, h.timeout) // set timeout

	go h.fetchSocksData(conn, remoteCon, target)

	return nil
}

func (h *udpHandler) fetchSocksData(conn core.UDPConn, remoteConn net.Conn, target *net.UDPAddr) {
	buf := core.NewBytes(core.BufSize)
	defer func() {
		core.FreeBytes(buf)
	}()

	n, err := remoteConn.Read(buf)
	if err != nil {
		log.Println(err, "read from socks failed")
		return
	}

	raw := buf[:n]
	_, _, payload, err := udppackage.UnpackUDPData(raw)
	if err != nil {
		log.Println(err, "unpack udp data failed!!")
		return
	}

	_, err = conn.WriteFrom(payload, target)
	if err != nil {
		log.Println(err, "write tun failed!!")
		return
	}

	if target.Port == dns.COMMON_DNS_PORT {
		h.dnsCache.Store(raw)
	}
}

// ReceiveTo will be called when data arrives from TUN.
func (h *udpHandler) ReceiveTo(conn core.UDPConn, data []byte, addr *net.UDPAddr) error {
	h.Lock()
	udpsocks, ok := h.udpSocks[conn]
	h.Unlock()
	if !ok {
		h.Close(conn)
		log.Println("can not find remote address <-->", conn.LocalAddr().String())
		return errors.New("can not find remote address")
	}

	if addr.Port == dns.COMMON_DNS_PORT {
		if answer := h.dnsCache.Query(data); answer != nil {
			var buf [1024]byte
			resp, _ := answer.PackBuffer(buf[:])
			_, err := conn.WriteFrom(resp, addr)
			if err != nil {
				h.Close(conn)
				log.Printf("write dns answer failed: %v", err)
				return errors.New("write remote failed")
			}
			return nil
		}
	}

	full, err := udppackage.PackUDPData(addr, conn.LocalAddr(), data)
	if err != nil {
		h.Close(conn)
		log.Println("pack udp data failed", err)
		return errors.New("pack udp data failed")
	}

	n, err := udpsocks.Write(full)
	if err != nil {
		h.Close(conn)
		log.Println("write to proxy failed", err)
		return errors.New("write to proxy failed")
	}
	log.Println("write bytes n", n)
	return nil
}

func (h *udpHandler) Close(conn core.UDPConn) {
	conn.Close()

	h.Lock()
	defer h.Unlock()
	if c, ok := h.udpSocks[conn]; ok {
		c.Close()
		delete(h.udpSocks, conn)
	}

}
