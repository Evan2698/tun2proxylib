//go:build !windows

package socketbase

import (
	"fmt"
	"log"
	"net"
	"syscall"

	"tun2proxylib/mobile" // 替换为你的真实包路径
)

func dialProtectedTCP(ip net.IP, port int, p mobile.ProtectSocket) (net.Conn, error) {
	// 1. 准备 sockaddr
	sa, err := netAddrToSockaddr(ip, port)
	if err != nil {
		log.Println("prepare sockaddr failed:", err)
		return nil, err
	}
	family := socketFamily(ip)

	// 2. 创建 socket
	fd, err := syscall.Socket(family, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	if err != nil {
		log.Println("create tcp socket failed:", err)
		return nil, err
	}
	// 必须在此关掉本地 fd，因为 net.FileConn 会 dup 一份新的 fd
	defer syscall.Close(fd)

	// 3. 保护 socket (防止流量走回 VPN/TUN)
	if p != nil {
		if ret := p.Protect(fd); ret != 0 {
			log.Println("protect tcp socket failed, code:", ret)
			return nil, fmt.Errorf("protect socket failed with code %d", ret)
		}
	}

	// 4. 设置 QoS 属性 (TOS / Traffic Class)
	if family == syscall.AF_INET6 {
		err = syscall.SetsockoptInt(fd, syscall.IPPROTO_IPV6, syscall.IPV6_TCLASS, 128)
	} else {
		err = syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_TOS, 128)
	}
	if err != nil {
		log.Println("set socket attributes failed:", err)
		return nil, err
	}

	// 5. Connect
	if err := syscall.Connect(fd, sa); err != nil {
		log.Println("tcp connect failed:", err)
		return nil, err
	}

	// 6. 转换为 net.Conn
	return fdToConn(uintptr(fd))
}
