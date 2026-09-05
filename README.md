# tun2proxylib

`tun2proxylib` 是一个 Go 库，用于读取 TUN 设备中的 IP 数据包，在用户态解析 TCP/UDP 流量，将流量转发到代理服务器，并把代理响应重新写回 TUN 设备。
项目提供两种协议栈实现：

- `gvisorcore`：基于 gVisor netstack，直接从 TUN 文件描述符创建用户态网络栈。
- `lwipcore`：通过 cgo 封装 lwIP，提供较底层的 TCP/UDP 回调接口。
目前只处理 TCP 和 UDP，不处理 ICMP 或其他传输协议。库本身不创建 TUN 设备，也不配置系统地址和路由，这些工作由调用方完成。

## 工作方式

TUN 设备 -> gVisor/lwIP 协议栈 -> TCP/UDP handler -> 代理服务器
	^                                             |
	+--------------------- 代理响应 --------------+
两种实现的主要区别：

| 实现 | 接入方式 | 适用场景 |
| --- | --- | --- |
| gVisor | `TransportHandler.HandleTCP` / `HandleUDP` | 直接从 TUN FD 创建链路端点，使用纯 Go 用户态协议栈 |
| lwIP | 注册 TCP/UDP handler，并通过输入输出回调交换数据 | 需要 cgo 和 C 编译器，使用 lwIP 协议栈 |
## 环境要求

- Go `1.25.1` 或兼容版本，版本要求见 [`go.mod`](go.mod)。
- 使用 gVisor 时，需要一个已打开且可读写的 TUN/TAP 文件描述符。
- 使用 lwIP 时，需要启用 CGO 和本机 C 编译器。
- Linux 上创建或配置 TUN 通常需要相应权限。
- TCP 代理使用 SOCKS5；gVisor 默认 UDP 代理使用项目自定义的 UDP 封包格式。

下载依赖：

```bash
go mod download
```

## 使用 gVisor

`CreateLinkEndpoint` 接收 TUN 文件描述符和 MTU，`CreateStack` 创建 IPv4/IPv6 协议栈，并将解析出的 TCP/UDP 连接交给 `TransportHandler`。

```go
package main

	"log"

	"tun2proxylib/gvisorcore"
	"tun2proxylib/gvisorcore/proxy"
)

func main() {
	// tunFD 应由调用方创建并配置。
	const tunFD = 3
	link, err := gvisorcore.CreateLinkEndpoint(tunFD, 1500)
	if err != nil {
		log.Fatal(err)
	}

	// TCPURL 是 SOCKS5 地址；UDPURL 对应理解 udppackage 格式的 UDP 服务。
	handler := proxy.NewDefaultProxy("127.0.0.1:1080", "127.0.0.1:1081", nil)
	_, err = gvisorcore.CreateStack(gvisorcore.StackOptions{
		TransportHandler: handler,
		LinkEndpoint:     link,
	})
	if err != nil {
		log.Fatal(err)
	}

	select {}
}
```

也可以实现自定义代理逻辑：

```go
type Handler struct{}

func (Handler) HandleTCP(conn gvisorcore.TCPConn) {
	defer conn.Close()
	// conn 实现 net.Conn，可在这里进行 TCP 双向转发。
}

func (Handler) HandleUDP(conn gvisorcore.UDPConn) {
	defer conn.Close()
	// 使用 conn.ReadFrom/WriteTo 进行 UDP 双向转发。
}
```

`gvisorcore/dialer` 提供 `DialContext` 和 `ListenPacket`，支持按网卡名称、网卡索引或 routing mark 设置出站 socket，可用于避免代理连接回流到 TUN。

### gVisor 默认代理

`gvisorcore/proxy.DefaultProxy` 的 TCP 侧通过 SOCKS5 建立连接，UDP 侧把数据编码后发送到指定 UDP 服务：

```go
handler := proxy.NewDefaultProxy(
	"127.0.0.1:1080", // TCP SOCKS5 服务
	"127.0.0.1:1081", // 自定义 UDP 服务
	protectSocket,     // 可为 nil；移动端可实现 mobile.ProtectSocket
)
```

UDP 服务端不能只实现标准 SOCKS5 UDP Associate，需要实现 [`udppackage`](udppackage/pack.go) 定义的封包协议。

## 使用 lwIP

lwIP 路径需要完成以下步骤：

1. 注册 `RegisterOutputFn`，把 lwIP 输出的 IP 包写回 TUN。
2. 注册 `RegisterTCPConnHandler` 和 `RegisterUDPConnHandler`。
3. 调用 `NewLWIPStack` 创建协议栈。
4. 从 TUN 读取数据并调用 `stack.Write`。
5. 退出时调用 `stack.Close`。

项目提供了 SOCKS5 TCP/UDP handler：

```go
dnsCache := cache.NewDNSCache()
core.RegisterTCPConnHandler(socks.NewTCPHandler("127.0.0.1", 1080))
core.RegisterUDPConnHandler(socks.NewUDPHandler(
	"127.0.0.1", 1081, 30*time.Second, dnsCache,
))

core.RegisterOutputFn(func(packet []byte) (int, error) {
	return tun.Write(packet) // tun 由调用方提供
})

stack := core.NewLWIPStack()
if stack == nil {
	log.Fatal("create lwIP stack failed")
}
defer stack.Close()

buf := make([]byte, 1500)
for {
	n, err := tun.Read(buf)
	if err != nil {
		log.Fatal(err)
	}
	if _, err = stack.Write(buf[:n]); err != nil {
		log.Fatal(err)
	}
}
```

上例中的 `tun`、`cache`、`core`、`socks`、`time` 和 `log` 需要按应用实际类型和导入路径补充。输出函数应在创建协议栈之前注册。

## UDP 封包格式

`udppackage.PackUDPData` 和 `UnpackUDPData` 用于项目内 UDP 代理数据交换：

```text
目标 IP 长度(1) | 目标 IP | 目标端口(2)
源 IP 长度(1)   | 源 IP   | 源端口(2)
payload 长度(4) | payload
```

长度字段为大端序，IP 长度使用 `net.IP` 的字节长度。代理服务端可使用 `UnpackUDPData` 解包，并使用 `PackUDPData` 生成响应。

## 目录说明

| 目录 | 作用 |
| --- | --- |
| `gvisorcore/` | gVisor netstack、TUN link endpoint、连接回调和出站 dialer |
| `gvisorcore/proxy/` | gVisor 默认 TCP/UDP 代理处理器 |
| `lwipcore/core/` | lwIP cgo 封装、连接对象、输入输出和 handler 注册 |
| `lwipcore/proxy/socks/` | lwIP 使用的 SOCKS5 TCP/UDP handler |
| `udppackage/` | UDP 代理数据的封包与解包 |
| `socketbase/` | 创建出站 socket，并支持移动端保护 socket |
| `mobile/` | 移动平台 socket 保护接口 |

## 生命周期和限制

- handler 通常在独立 goroutine 中处理连接，调用方应负责关闭连接和协议栈。
- TUN MTU 应与 `CreateLinkEndpoint` 的 MTU 以及读写缓冲区保持一致，常用值为 `1500`。
- Android 等移动平台应使用 `mobile.ProtectSocket` 保护代理 socket，避免代理连接再次进入 TUN；桌面环境可以传 `nil`。
- lwIP 依赖 cgo，交叉编译时需要目标平台的 C 工具链和头文件。
- 项目没有提供完整的 TUN 创建程序、代理服务端或命令行入口，需要由宿主应用补齐。
- gVisor 默认 UDP handler 使用自定义封包格式，不是标准 SOCKS5 UDP Associate。

## 测试

运行全部测试：

```bash
go test ./...
```

当前仓库的 lwIP 路径可能因 [`lwipcore/core/errors.go`](lwipcore/core/errors.go) 中已有的整数错误码字符串转换问题无法通过 Go 编译检查。gVisor、`socketbase` 和 `udppackage` 可单独验证：

```bash
go test ./gvisorcore/... ./socketbase ./udppackage
```

## 依赖与来源

项目使用 gVisor netstack、lwIP（通过 cgo 集成）以及 `golang.org/x/net/proxy`。lwIP 集成代码部分参考了已归档的 [`eycorsican/go-tun2socks`](https://github.com/eycorsican/go-tun2socks)，SOCKS 相关实现参考了 [`nadoo/glider`](https://github.com/nadoo/glider) 和 [`shadowsocks/go-shadowsocks2`](https://github.com/shadowsocks/go-shadowsocks2)。具体依赖版本以 [`go.mod`](go.mod) 为准。
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



