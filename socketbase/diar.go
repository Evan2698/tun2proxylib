package socketbase

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"syscall"
	"tun2proxylib/mobile"
)

func TcpDail(IP net.IP, port int, p mobile.ProtectSocket) (net.Conn, error) {
	if p == nil {
		conn, err := net.Dial("tcp", net.JoinHostPort(IP.String(), strconv.Itoa(port)))
		return conn, err
	}

	//1. prepare address
	sa, err := netAddrToSockaddr(IP, port)
	if err != nil {
		log.Println("prepare sockaddr failed!!!", err)
		return nil, err
	}

	//2. create socket
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	if err != nil {
		log.Println("create tcp socket failed!!!", err)
		return nil, err
	}
	// release fd, because net.FileConn will dup it
	defer syscall.Close(fd)

	//3. protect socket
	ret := p.Protect(fd)
	if ret != 0 {
		log.Println("protect tcp socket failed!!!", ret)
		return nil, syscall.EINVAL
	}

	//4. set attribute
	err = syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_TOS, 128)
	if err != nil {
		log.Println("set socket attributes ", err)
		return nil, err
	}

	//5. connect
	err = syscall.Connect(fd, sa)
	if err != nil {
		log.Println("tcp connect failed!!!", err)
		return nil, err
	}

	//6. convert fd to net.Conn
	return fdToConn(uintptr(fd))
}

func UdpDail(IP net.IP, port int, p mobile.ProtectSocket) (net.Conn, error) {
	if p == nil {
		conn, err := net.Dial("udp", net.JoinHostPort(IP.String(), strconv.Itoa(port)))
		return conn, err
	}

	//1. prepare address
	sa, err := netAddrToSockaddr(IP, port)
	if err != nil {
		log.Println("prepare sockaddr failed!!!", err)
		return nil, err
	}

	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP)
	if err != nil {
		log.Println("create udp socket failed!!!", err)
		return nil, err
	}
	defer syscall.Close(fd)

	//3. protect socket
	ret := p.Protect(fd)
	if ret != 0 {
		log.Println("protect tcp socket failed!!!", ret)
		return nil, syscall.EINVAL
	}

	//4. connect
	err = syscall.Connect(fd, sa)
	if err != nil {
		log.Println("tcp connect failed!!!", err)
		return nil, err
	}

	//5. convert fd to net.Conn
	return fdToConn(uintptr(fd))
}

func TcpDailNetString(netString string, p mobile.ProtectSocket) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(netString)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf("parse ip failed")
	}
	return TcpDail(ip, port, p)
}
func UdpDailNetString(netString string, p mobile.ProtectSocket) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(netString)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf("parse ip failed")
	}
	return UdpDail(ip, port, p)
}

func netAddrToSockaddr(ip net.IP, port int) (syscall.Sockaddr, error) {
	if ip.To4() != nil {
		var addr [4]byte
		copy(addr[:], ip.To4())
		sa := &syscall.SockaddrInet4{
			Port: port,
			Addr: addr,
		}
		return sa, nil
	} else if ip.To16() != nil {
		var addr [16]byte
		copy(addr[:], ip.To16())
		sa := &syscall.SockaddrInet6{
			Port: port,
			Addr: addr,
		}
		return sa, nil
	} else {
		return nil, fmt.Errorf("convert net address error")
	}
}

func fdToConn(fd uintptr) (net.Conn, error) {
	// 1. FD -> *os.File
	// os.NewFile(fd uintptr, name string) *os.File
	// 第一个参数是文件描述符，第二个参数是给文件起的名字（仅用于调试/显示）。
	// 在 Unix/Linux 上，通常将 uintptr 转换为 int 传递给 os.NewFile。
	// 注意：os.NewFile 返回的 *os.File **接管了**原始文件描述符的生命周期，
	// 当这个 os.File 被关闭时，原始文件描述符也会被关闭。
	file := os.NewFile(fd, fmt.Sprintf("socket-%d", fd))
	if file == nil {
		return nil, fmt.Errorf("os.NewFile returned nil for fd %d", fd)
	}
	defer file.Close() // 重要的：os.NewFile返回的文件必须关闭，但我们将其传递给 net.FileConn

	// 2. *os.File -> net.Conn
	// net.FileConn(f *os.File) (c net.Conn, err error)
	// net.FileConn 会复制 (dup) 原始的文件描述符，然后基于新的文件描述符创建一个 net.Conn。
	// 这意味着关闭新的 net.Conn 不会影响原始的 os.File (以及它的 FD)，反之亦然。
	conn, err := net.FileConn(file)
	if err != nil {
		return nil, fmt.Errorf("net.FileConn error: %w", err)
	}

	return conn, nil
}
