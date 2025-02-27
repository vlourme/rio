# RIO

基于`IOURING`的网络库，且遵循标准库模式。支持协议：`TCP`、`UDP`、`UNIX`（`IP`为代理标准库）。

Linux 内核版本需要`>= 5.4`，推荐版本为`>= 6.1`。

## 使用

```shell
go get -u github.com/brickingsoft/rio
```

基本使用`rio`替换`net`：
```go

// 将 net.Listen() 替换成 rio.Listen() 
ln, lnErr := rio.Listen("tcp", ":9000")

```

TLS场景：
```go
ln, lnErr := rio.Listen("tcp", ":9000")
// check err
ln, lnErr := tls.NewListener(ln, config)
```

转换场景：
```go
// tcp sendfile
reader, ok := conn.(io.ReaderFrom)
// 转换成 TCP 链接 
tcpConn, ok := conn.(*rio.TCPConn)
// 转换成 UDP 链接
udpConn, ok := conn.(*rio.UDPConn)
// 转换成 UNIX 链接
unixConn, ok := conn.(*rio.UnixConn)
```

纯客户端场景：

建议`PIN`住`IOURING`，直到程序退出再`UNPIN`。

因为`IOURING`的生命周期为当被使用时开启，当被没有被使用时关闭。

因为`Listen`的生命周期往往和程序是一致的，所以`IOURING`为常驻状况。

而`Dial`的生命周期是短的，往往是频繁`Dial`，所以需要`PIN`来常驻`IOURING`，而不是频繁开闭。
```go
// 程序启动位置
pinErr := rio.PinVortexes()
// 程序退出位置
unpinErr := rio.UnpinVortexes()
```

## 调配 IOURING 参数
`IOURING`有不少参数可以设置，具体见 [setup](https://manpages.debian.org/unstable/liburing-dev/io_uring_enter.2.en.html)。

因为`IOURING`是 Linux 专有的。所以需要`go:build linux`指定构建。

```go
//go:build linux

package foo

import (
	"github.com/brickingsoft/rio"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
)

func Setup() {
	// 开启 zerocopy 发送，内核版本需要高于6.0。
	rio.UseZeroCopy(true)
	// 启用 preform 模式，启用 SetupSQPoll|SetupSingleIssuer 等选项。
	// preform 需要很多 CPU 资源，酌情选择。
	// 默认 default 模式就已经根据内核版本开启能开启的优化选项。
	rio.UsePreformMode()
	// 设置 ring 的大小，默认为 16384，也是推荐最大值。
	rio.UseEntries(1024)
	// 设置 ring 边缘数量。
	// 总默认数量为 cpu 数。第一个为中心环，其余为边缘环。
	// 中心环用于处理 Listen 和 Dial。
	// 边缘环用于处理 Conn 的 io 操作。
	rio.UseSides(5)
	// 设置边缘环的加载平衡器。
	// aio.RoundRobinLoadBalancer{}
	// aio.RandomLoadBalancer{}
	// aio.LeastLoadBalancer{}
	rio.UseSidesLoadBalancer(lb)
	// 设置等待变速器构建器。
	// 
	rio.UseWaitTransmissionBuilder(builder)
	//
	rio.UseReadFromFilePolicy(rio.ReadFromFileUseMMapPolicy)
}

```


