# tun2proxylib

`tun2proxylib` 是一个 Go 库，用于读取 TUN 设备中的 IP 数据包，将 TCP/UDP
流量交给用户提供的代理处理器，并把代理响应重新写回 TUN 设备。

项目目前支持两种协议栈实现：

- `gvisorcore`：基于 gVisor netstack，适合直接从 TUN 文件描述符创建用户态网络栈。
- `lwipcore`：通过 cgo 封装 lwIP，提供更底层的 TCP/UDP 回调接口。

项目只处理 TCP 和 UDP，不处理 ICMP 或其他传输协议。它本身不是一个完整的
TUN 创建工具，也不负责配置系统路由；TUN 设备、地址、路由和文件描述符需要由
调用方或宿主应用准备好。

## 工作方式

`tun2proxylib` 是一个 Go 库，用于从 TUN 设备读取 IP 数据包，在用户态解析 TCP/UDP 流量，并将流量转发到代理服务器。它适合用于 VPN、隧道、代理客户端以及 Android 等需要自行处理 TUN 文件描述符的场景。
TUN 设备 -> gVisor/lwIP 协议栈 -> TCP/UDP handler -> 代理服务器
目前支持：

- IPv4 和 IPv6 的 TCP 转发
- IPv4 和 IPv6 的 UDP 转发
- SOCKS5 TCP 连接建立（通过 `golang.org/x/net/proxy`）
- 面向 UDP 代理的自定义封包格式，见 [`udppackage/pack.go`](udppackage/pack.go)
- gVisor netstack 和 lwIP 两种用户态协议栈

目前不支持直接转发 ICMP 或其他非 TCP/UDP 协议。库本身也不负责创建、配置或销毁 TUN 设备，调用方需要提供已经打开的 TUN 文件描述符。

两条实现的接入边界不同：

- gVisor 使用 `TransportHandler` 的 `HandleTCP` 和 `HandleUDP` 回调。
- lwIP 使用 `RegisterTCPConnHandler`、`RegisterUDPConnHandler` 注册处理器，
  使用 `RegisterOutputFn` 把协议栈输出写回 TUN，并通过 `LWIPStack.Write` 注入 TUN
  读到的数据。

## 环境要求

- Go `1.25.1` 或兼容版本，模块名为 `tun2proxylib`。
两种实现的选择：

| 实现 | 入口 | 特点 |
| --- | --- | --- |
| gVisor | `tun2proxylib/gvisorcore` | 纯 Go 用户态协议栈，适合直接从 TUN FD 创建链路端点；通过 `TransportHandler` 接收连接 |
| lwIP | `tun2proxylib/lwipcore/core` | 通过 cgo 包装 lwIP；需要注册输入输出回调和 TCP/UDP handler |

- 使用 `lwipcore` 时需要启用 cgo 和本机 C 编译器。
- 使用 gVisor 时需要一个已经打开的 TUN 文件描述符；Linux 下通常需要相应的

```bash
go mod download
go test ./...
```

当前仓库的 lwIP 包存在一个会被 `go vet` 报告的既有问题：
`lwipcore/core/errors.go` 将错误码直接转换为 `string`。因此在修复该问题前，
`go test ./...` 可能会在编译 lwIP 包时失败；gVisor、socket 和 UDP 封包相关包
可以单独测试。

## 使用 gVisor 协议栈

`gvisorcore.CreateLinkEndpoint` 接收 TUN 的文件描述符和 MTU，
`CreateStack` 负责创建 IPv4/IPv6 协议栈，并将 TCP/UDP 连接交给处理器。

下面的示例使用项目内置的默认处理器：TCP 通过 SOCKS5 代理发送；UDP 使用项目
定义的 UDP 封包格式发送到 `udpURL` 指定的 UDP 服务。

```go
package main

import (
	"log"

	"tun2proxylib/gvisorcore"
	"tun2proxylib/gvisorcore/proxy"
)

func main() {
	// tunFD 应由调用方创建并配置，例如 Linux TUN/TAP 的文件描述符。
	const tunFD = 3
	linkEndpoint, err := gvisorcore.CreateLinkEndpoint(tunFD, 1500)
	if err != nil {
		log.Fatal(err)
	}

	// TCPURL 是 SOCKS5 服务地址；UDPURL 是接收 udppackage 格式数据的 UDP 服务。
	handler := proxy.NewDefaultProxy(
		"127.0.0.1:1080",
		"127.0.0.1:1081",
		nil, // 移动端可传入 mobile.ProtectSocket 实现
	)

	_, err = gvisorcore.CreateStack(gvisorcore.StackOptions{
		TransportHandler: handler,
		LinkEndpoint:     linkEndpoint,
	})
	if err != nil {
		log.Fatal(err)
	}

	// 保持进程运行；协议栈会在后台处理 TUN 数据。
	select {}
}
```

如果需要自定义代理逻辑，实现 `gvisorcore.TransportHandler`：

```go
type Handler struct{}

func (Handler) HandleTCP(conn gvisorcore.TCPConn) {
	// 读取 conn，并将数据转发到 TCP 代理；结束时关闭 conn。
}

func (Handler) HandleUDP(conn gvisorcore.UDPConn) {
	// 使用 conn.ReadFrom/WriteTo 与 UDP 代理交换数据。
}
```

`gvisorcore/dialer` 提供 `DialContext` 和 `ListenPacket`，可按网卡、网卡索引
或 routing mark 设置出站 socket，适合需要避免代理连接回流到 TUN 的场景。

## 使用 lwIP 协议栈

lwIP 的调用方需要完成三件事：

1. 从 TUN 读取数据并调用 `LWIPStack.Write`。
2. 注册 `RegisterOutputFn`，把 lwIP 输出的 IP 包写回 TUN。
3. 注册 TCP/UDP handler，负责与代理建立连接并交换数据。

项目提供了 SOCKS5 handler：

```go
package main

import (
	"log"
	"time"

	"tun2proxylib/lwipcore/common/dns/cache"
	"tun2proxylib/lwipcore/core"
	"tun2proxylib/lwipcore/proxy/socks"
)

func main() {
	dnsCache := cache.NewDNSCache()
	core.RegisterTCPConnHandler(socks.NewTCPHandler("127.0.0.1", 1080))
	core.RegisterUDPConnHandler(socks.NewUDPHandler(
		"127.0.0.1", 1080, 30*time.Second, dnsCache,
	))

	stack := core.NewLWIPStack()
	if stack == nil {
		log.Fatal("create lwIP stack failed")
	}
	defer stack.Close()

	// tun 是调用方准备好的 io.ReadWriter。
	// core.RegisterOutputFn(func(data []byte) (int, error) {
	//     return tun.Write(data)
	// })
	// for {
	//     n, err := tun.Read(buf)
	//     if err != nil { log.Fatal(err) }
	//     if _, err = stack.Write(buf[:n]); err != nil { log.Fatal(err) }
	// }
}
```

实际使用时应在创建协议栈前注册输出函数，并在独立的读循环中持续向
`stack.Write` 注入 TUN 数据。程序退出时调用 `Close`，停止计时器并关闭已有
连接。

## UDP 封包格式

`udppackage.PackUDPData` 和 `UnpackUDPData` 用于项目内 UDP 代理数据交换。一个
数据包依次包含：目标 IP 长度与地址、目标端口、源 IP 长度与地址、源端口、4 字节
大端 payload 长度和 payload。IP 长度字段使用 `net.IP` 的字节长度，因此代理服务
端应按该格式进行封包和解包。

## 目录说明

| 目录 | 作用 |
| --- | --- |
| `gvisorcore/` | gVisor netstack、TUN link endpoint、连接回调和出站 dialer |
| `gvisorcore/proxy/` | gVisor 默认 TCP/UDP 代理处理器 |
| `lwipcore/core/` | lwIP cgo 封装、连接对象、输入输出和 handler 注册 |
| `lwipcore/proxy/socks/` | lwIP 使用的 SOCKS5 TCP/UDP handler |
| `udppackage/` | UDP 代理数据的封包与解包 |

- 当前只转发 TCP 和 UDP。
- 不负责创建 TUN、设置地址、配置路由或启动代理服务。

## 来源

lwIP 相关实现参考了已归档的
[`eycorsican/go-tun2socks`](https://github.com/eycorsican/go-tun2socks)。



