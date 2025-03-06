# RIO

基于`IOURING`的`AIO`网络库，非`CGO`方式，且遵循标准库使用设计模式。

支持协议：`TCP`、`UDP`、`UNIX`、`UNIXGRAM`（`IP`为代理标准库）。

Linux 内核版本需要`>= 5.14`，推荐版本为`>= 6.1`。

## 性能
### Benchmark
测试环境：Win11（WSL2）、内核（6.6.36.6-microsoft-standard-WSL2）、CPU（13600K）。

基于默认参数的测试，10线程，每线程1000链接，共计10000链接。

RIO 相比 STD（go net 标准库）约快 `13%`，详见 [Benchmark](https://github.com/brickingsoft/rio_examples/tree/main/benchmark) 。

注意：CurveWaitTransmission 在不同环境下的性能体现是不同的，需按需调整来发挥出高效的性能。 


<img src="benchmark/echo.png" width="336" height="144" border="0" alt="echo benchmark">
<img src="benchmark/http.png" width="336" height="144" border="0" alt="http benchmark">


```text
------ Benchmark ------
Port: 9000
Workers: 10
Count: 1000
NBytes: 1024
ECHO-RIO benching complete(1.564700361s): 6391 conn/sec, 6.2M inbounds/sec, 6.2M outbounds/sec, 0 failures
ECHO-STD benching complete(1.821161901s): 5491 conn/sec, 5.4M inbounds/sec, 5.4M outbounds/sec, 0 failures
HTTP-RIO benching complete(1.722059583s): 5807 conn/sec, 5.8M inbounds/sec, 5.8M outbounds/sec, 0 failures
HTTP-STD benching complete(1.948937829s): 5131 conn/sec, 5M inbounds/sec, 5M outbounds/sec, 0 failures
```

### TCPKALI

服务端环境：Win11（Hyper-V）、Ubuntu24.10（6.11.0-8-generic）、CPU（4核）。

客户端环境：Win11（WSL2）、内核（6.6.36.6-microsoft-standard-WSL2）、CPU（13600K）。

`tcpkali` 压测结果为 RIO 相比 STD（go net 标准库）约快 `30%` 。

注意：请不要本地压测本地。

<img src="benchmark/tcpkali-echo.png" width="336" height="144" border="0" alt="http benchmark">

| 类型  | packet rate estimate |
|-----|----------------------|
| RIO | 22330.7              |
| NET | 16918.8              |

```shell
tcpkali --workers 1 -c 50 -T 10s -m "PING" 192.168.100.120:9000
```
```text
------ RIO ------
Destination: [192.168.100.120]:9000
Interface eth0 address [192.168.100.1]:0
Using interface eth0 to connect to [192.168.100.120]:9000
Ramped up to 50 connections.
Total data sent:     218.8 MiB (229455392 bytes)
Total data received: 217.3 MiB (227831216 bytes)
Bandwidth per channel: 7.316⇅ Mbps (914.5 kBps)
Aggregate bandwidth: 182.254↓, 183.554↑ Mbps
Packet rate estimate: 22330.7↓, 15969.6↑ (3↓, 35↑ TCP MSS/op)
Test duration: 10.0006 s.
```
```text
------ NET ------
Destination: [192.168.100.120]:9000
Interface eth0 address [192.168.100.1]:0
Using interface eth0 to connect to [192.168.100.120]:9000
Ramped up to 50 connections.
Total data sent:     217.6 MiB (228130816 bytes)
Total data received: 215.8 MiB (226292180 bytes)
Bandwidth per channel: 7.122⇅ Mbps (890.2 kBps)
Aggregate bandwidth: 180.871↓, 182.341↑ Mbps
Packet rate estimate: 16918.8↓, 15884.2↑ (2↓, 45↑ TCP MSS/op)
Test duration: 10.009 s.
```


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

而`Dial`的生命周期是短的，往往是频繁`Dial`，所以需要`PIN`来常驻`IOURING`，而不是频繁启停。
```go
// 程序启动位置
rio.Pin()
// 程序退出位置
rio.Unpin()
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

REUSE PORT（监听TCP时自动启用）：

```go

lc := rio.ListenConfig{}
lc.SetReusePort(true)

ln, lnErr := lc.Listen(...)

```

## 进阶调参
通过设置环境变量进行调控，具体详见 [IOURING](https://man7.org/linux/man-pages/man2/io_uring_setup.2.html)。

| 名称                         | 值  | 说明                                                   |
|----------------------------|----|------------------------------------------------------|
| IOURING_ENTRIES            | 数字 | 环大小，默认为最大值 16384。                                    |
| IOURING_SETUP_FLAGS        | 文本 | 标识，如`IORING_SETUP_SQPOLL, IORING_SETUP_SUBMIT_ALL`等。 |
| IOURING_SQ_THREAD_CPU      | 数字 | 设置环锁亲和的CPUID。                                        |
| IOURING_SQ_THREAD_IDLE     | 数字 | 在含有`IORING_SETUP_SQPOLL`标识时，设置空闲时长，单位为毫秒，默认是 1 毫秒。   |
| IOURING_PREPARE_BATCH_SIZE | 数字 | 准备 SQE 的缓冲大小，默认为 SQ 的大小。                             |
| IOURING_USE_CPU_AFFILIATE  | 布尔 | 是否使用 CPU AFFILIATE。                                  |
| IOURING_CURVE_TRANSMISSION | 文本 | 设置等待 CQ 策略曲线，如 `1:1us, 8:1us`。                       |

注意：`IOURING_SETUP_FLAGS` 与系统内核版本有关联，如果使用 `NONE` 方案进行自定义时，请务必确认版本。



