# RIO

基于`IOURING`的网络库，且遵循标准库模式，且非`CGO`方式。

支持协议：`TCP`、`UDP`、`UNIX`、`UNIXGRAM`（`IP`为代理标准库）。

Linux 内核版本需要`>= 5.4`，推荐版本为`>= 6.1`。

注意：相比其它 `IOURING` 库，它更适合企业生产。
它遵循标准库模式，所以项目无需进行大改，几乎没有侵入，支持 `TLS`。
可能其它 `IOURING` 库性能会更高一些，
因为它们往往不是面向`Conn`，而是面向`Data`。当补上`Conn`，这势必需要带锁的竞争容器。
`Conn`这往往是必须的，因为解析流里的数据这步正常都会有。
二是缺少异步转同步的这一层，这意味着需要在有`goroutine`的情况下进行异步编程。
而`goroutine`的价值之一是将面向异步编程变为面向同步编程。
面向异步编程在`GO`中是稀有的。
所以综合之后，`RIO`会更快，因为它使用指针传递（线性安全）来代替锁来实现面向`Conn`编程。

## 使用

```shell
go get -u github.com/brickingsoft/rio
```

基本使用`rio`替换`net`：
```go

// 将 net.Listen() 替换成 rio.Listen() 
ln, lnErr := rio.Listen("tcp", ":9000")
// 将 net.Dial() 替换成 rio.Dial() 
conn, dialErr := rio.Dial("tcp", "127.0.0.1:9000")

```

TLS场景：
```go
// server("github.com/brickingsoft/rio/tls")
ln, _ = tls.Listen("tcp", "127.0.0.1:9000", tls.ConfigFrom(config))
// server(use wrap)
ln, _ := rio.Listen("tcp", ":9000")
ln, _ := tls.NewListener(ln, config)

// client("github.com/brickingsoft/rio/tls")
conn, _ = tls.Dial("tcp", "127.0.0.1:9000", tls.ConfigFrom(config))

// client(use wrap)
rawConn, dialErr := rio.Dial("tcp", "127.0.0.1:9000")
conn := tls.Client(rawConn, config)
if err := conn.HandshakeContext(ctx); err != nil {
	rawConn.Close()
	return nil, err
}
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

HTTP场景：

Server 使用`Listener`代替法。

Client 使用`RoundTripper`代替法。
```go
// http server
http.Serve(ln, handler)
// fasthttp server
fasthttp.Serve(ln, handler)
```

## 调配 IOURING 参数
`IOURING`有不少参数可以设置，具体见 [setup](https://manpages.debian.org/unstable/liburing-dev/io_uring_enter.2.en.html)。

因为`IOURING`是 Linux 专有的。所以需要`go:build linux`指定构建。

```go
//go:build linux

package foo

import (
	"github.com/brickingsoft/rio"
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
	// 设置完成事件等待变速器构建器。
	// 默认为曲线变速器构建器，支持自定义曲线。
	rio.UseWaitTransmissionBuilder(builder)
	// 设置从文件读取的策略。
	// 默认是 mmap，可使用 splice+mmap 的混合策略。
	// 因 iouring 不支持 sendfile，则使用 mmap 的方式。
	// 因 splice 对大文件不友好，对小文件支持可以，所以增加混合策略。
	// 混合策略为当文件尺寸小于输出尺寸时，使用 splice，大于后使用 mmap。
	// 注意！！！ 混合策略不是安全的。
	rio.UseReadFromFilePolicy(rio.ReadFromFileUseMMapPolicy)
}

```

```go
//go:build !linux

package foo

func Setup() {}

```

```go

func main() {
	Setup() // 在 pin 之前。
}

```
