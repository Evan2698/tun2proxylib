package core

/*
#cgo CFLAGS: -I./c/include
#include "lwip/udp.h"
*/
import "C"
import (
	"log"
	"time"
	"unsafe"
)

//export udpRecvFn
func udpRecvFn(arg unsafe.Pointer, pcb *C.struct_udp_pcb, p *C.struct_pbuf, addr *C.ip_addr_t, port C.u16_t, destAddr *C.ip_addr_t, destPort C.u16_t) {
	defer func() {
		if p != nil {
			C.pbuf_free(p)
		}
	}()

	UdpOnce.Do(destoryWithTimeout)

	if pcb == nil {
		return
	}

	srcAddr := ParseUDPAddr(ipAddrNTOA(*addr), uint16(port))
	dstAddr := ParseUDPAddr(ipAddrNTOA(*destAddr), uint16(destPort))
	if srcAddr == nil || dstAddr == nil {
		log.Print("invalid UDP address")
		return
	}

	connId := udpConnId{
		src: srcAddr.String(),
	}
	conn, found := udpConns.Load(connId)
	if !found {
		if udpConnHandler == nil {
			log.Print("must register a UDP connection handler")
			return
		}
		var err error
		conn, err = newUDPConn(pcb,
			udpConnHandler,
			*addr,
			port,
			srcAddr,
			dstAddr)
		if err != nil {
			return
		}
		udpConns.Store(connId, conn)

	}

	Udplock.Lock()
	UdpConMap.Store(conn, timeout)
	Udplock.Unlock()

	var buf []byte
	var totlen = int(p.tot_len)
	if p.tot_len == p.len {
		buf = (*[1 << 30]byte)(unsafe.Pointer(p.payload))[:totlen:totlen]
	} else {
		buf = NewBytes(totlen)
		defer FreeBytes(buf)
		C.pbuf_copy_partial(p, unsafe.Pointer(&buf[0]), p.tot_len, 0)
	}

	conn.(UDPConn).ReceiveTo(buf[:totlen], dstAddr)
}

func destoryWithTimeout() {

	go func() {

		ticker := time.NewTicker(5 * time.Second)
		for {
			<-ticker.C
			onAction()
			if Exit {
				break
			}
		}
	}()

}

func onAction() {
	Udplock.Lock()
	defer Udplock.Unlock()

	UdpConMap.Range(func(key, value interface{}) bool {
		conn, ok := key.(UDPConn)
		if !ok {
			return true
		}
		t, ok := value.(int)
		if !ok {
			return true
		}
		if t <= 0 {
			conn.Close()
			UdpConMap.Delete(conn)
			udpConns.Delete(udpConnId{src: conn.LocalAddr().String()})
		} else {
			UdpConMap.Store(conn, t-1)
		}
		return true
	})
}
