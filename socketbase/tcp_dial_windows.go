package socketbase

import (
	"net"
	"strconv"

	"tun2proxylib/mobile" // 替换为你的真实包路径
)

func dialProtectedTCP(ip net.IP, port int, p mobile.ProtectSocket) (net.Conn, error) {
	// Windows 上不存在 Android/iOS 类型的 ProtectSocket 机制，
	// 回退至标准的 net.Dialer 实现以确保良好的跨平台兼容性。
	dialer := &net.Dialer{}
	address := net.JoinHostPort(ip.String(), strconv.Itoa(port))
	return dialer.Dial("tcp", address)
}
