# tun2proxylib 
是一个将tun 设备中流量解析出来，并发送到代理服务器上，目前只是转发TCP 和 UDP，其他协议不支持。

这个lib 中包含两种解析tun 的方式：
* 使用lwip用golang 的cgo 包装一下，这部分代码是https://github.com/eycorsican/go-tun2socks 出自这部分代码，因为这部分代码源码作者已经将其归档了。这部分代码可以用，我在现实项目中使用测试过。
* 第二部分使用gvisor中的netstak 部分，好用，香。

