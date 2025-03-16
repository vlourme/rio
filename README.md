# RIO

基于`IOURING`的`AIO`网络库，非`CGO`方式，且遵循标准库使用设计模式。

支持协议：`TCP`、`UDP`、`UNIX`、`UNIXGRAM`（`IP`为代理标准库）。

***Linux 内核版本需要`>= 5.19`，推荐版本为`>= 6.6`。***

## 性能
### 对比测试
使用 `tcpkali` 进行压力测试，`RIO` 使用默认设置方案。

[基准测试代码地址](https://github.com/brickingsoft/rio_examples/tree/main/benchmark) 

```shell
tcpkali --workers 1 -c 50 -T 10s -m "PING" 192.168.100.120:9000
```

| 端   | 平台      | IP              | OS                                             | 规格     |
|-----|---------|-----------------|------------------------------------------------|--------|
| 客户端 | WSL2    | 192.168.100.1   | Ubuntu22.04 （6.6.36.6-microsoft-standard-WSL2） | 4C 16G |
| 服务端 | Hyper-V | 192.168.100.120 | Ubuntu24.10（6.11.0-8-generic）                  | 4C 8G  |


<img src="benchmark/bench_echo.png" width="336" height="144" border="0" alt="http benchmark">

| 种类           | 速率 （pps） | 说明       | 性能    |
|--------------|----------|----------|-------|
| RIO(DEFAULT) | 35599.0  | 稳定在35000 | 100 % |
| EVIO         | 18568.5  | 稳定在18000 | 52 %  |
| GNET         | 17832.6  | 稳定在17000 | 50 %  |
| NET          | 14937.1  | 稳定在14000 | 42 %  |

<details>
<summary>详细结果</summary>

```text
------ RIO(DEFAULT) ------
Destination: [192.168.100.120]:9000
Interface eth0 address [192.168.100.1]:0
Using interface eth0 to connect to [192.168.100.120]:9000
Ramped up to 50 connections.
Total data sent:     368.5 MiB (386415368 bytes)
Total data received: 366.7 MiB (384519186 bytes)
Bandwidth per channel: 12.325⇅ Mbps (1540.7 kBps)
Aggregate bandwidth: 307.375↓, 308.891↑ Mbps
Packet rate estimate: 35599.0↓, 26694.1↑ (3↓, 26↑ TCP MSS/op)
Test duration: 10.0078 s.
```

```text
------ EVIO ------
Destination: [192.168.100.120]:9000
Interface eth0 address [192.168.100.1]:0
Using interface eth0 to connect to [192.168.100.120]:9000
Ramped up to 50 connections.
Total data sent:     177.4 MiB (185991168 bytes)
Total data received: 176.0 MiB (184593536 bytes)
Bandwidth per channel: 5.925⇅ Mbps (740.6 kBps)
Aggregate bandwidth: 147.555↓, 148.673↑ Mbps
Packet rate estimate: 18568.5↓, 12776.1↑ (3↓, 44↑ TCP MSS/op)
Test duration: 10.0081 s.
```

```text
------ GNET ------
Destination: [192.168.100.120]:9000
Interface eth0 address [192.168.100.1]:0
Using interface eth0 to connect to [192.168.100.120]:9000
Ramped up to 50 connections.
Total data sent:     176.8 MiB (185401344 bytes)
Total data received: 175.4 MiB (183927028 bytes)
Bandwidth per channel: 5.908⇅ Mbps (738.4 kBps)
Aggregate bandwidth: 147.099↓, 148.278↑ Mbps
Packet rate estimate: 17832.6↓, 12716.7↑ (3↓, 44↑ TCP MSS/op)
Test duration: 10.0029 s.
```

```text
------ NET ------
Destination: [192.168.100.120]:9000
Interface eth0 address [192.168.100.1]:0
Using interface eth0 to connect to [192.168.100.120]:9000
Ramped up to 50 connections.
Total data sent:     198.3 MiB (207945728 bytes)
Total data received: 196.6 MiB (206165284 bytes)
Bandwidth per channel: 6.623⇅ Mbps (827.9 kBps)
Aggregate bandwidth: 164.859↓, 166.282↑ Mbps
Packet rate estimate: 14937.1↓, 14506.0↑ (2↓, 45↑ TCP MSS/op)
Test duration: 10.0045 s.
```
</details>


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
// 内置模式
// server("github.com/brickingsoft/rio/security")
ln, _ = security.Listen("tcp", ":9000", config)

// client("github.com/brickingsoft/rio/security")
conn, _ = security.Dial("tcp", "127.0.0.1:9000", config)

// 包裹模式
// server(use crypto/tls wrap)
ln, _ := rio.Listen("tcp", ":9000")
ln, _ := tls.NewListener(ln, config)

// client(use crypto/tls wrap)
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

FIXED READ AND WRITE：

使用注册的`BUFFER`进行读写，这会减少用户与内核之间的切换。

必须设置`IOURING_REG_BUFFERS`环境变量来注册`BUFFER`，详细见`进阶调参`。


```go
frw, ok := rio.Fixed(conn)
if !ok {
	// handle err
}

b := make([]byte, 1024)
buf := frw.AcquireRegisteredBuffer() // acquire buffer

if buf == nil { // means not registered or no buffer remains
	// handle no buf
}

// read
rn, rErr := frw.ReadFixed(buf)
// check err
_, _ = buf.Read(b[:rn]) // read into b

// write
buf.Reset() // reset is good

_, _ = buf.Write(b[:rn]) // write into buffer
wn, wErr := frw.WriteFixed(buf)
// check err

frw.ReleaseRegisteredBuffer(buf) // must release buffer 

```

## 进阶使用

### TLS


### Zero-copy

### TCP


### Fixed Buffer


### Fixed Fd

### 参数设置
通过设置环境变量进行调控，具体详见 [IOURING](https://man7.org/linux/man-pages/man2/io_uring_setup.2.html)。

| 名称                             | 值  | 说明                                                 |
|--------------------------------|----|----------------------------------------------------|
| RIO_IOURING_ENTRIES            | 数字 | 环大小，默认为最大值 16384。                                  |
| RIO_IOURING_SETUP_FLAGS        | 文本 | 标识，如`IORING_SETUP_SQPOLL, IORING_SETUP_SQ_AFF`等。   |
| RIO_IOURING_SETUP_FLAGS_SCHEMA | 文本 | 标识方案，`DEFAULT` 或 `PERFORMANCE`。                    |
| RIO_IOURING_SQ_THREAD_CPU      | 数字 | 设置 SQ 环锁亲和的 CPU。                                   |
| RIO_IOURING_SQ_THREAD_IDLE     | 数字 | 在含有`IORING_SETUP_SQPOLL`标识时，设置空闲时长，单位为毫秒，默认是 10 秒。 |
| RIO_IOURING_REG_FIXED_BUFFERS  | 文本 | 设置注册固定字节缓存，格式为 `单个大小, 个数`， 如`4096, 100`。           |
| RIO_IOURING_REG_FIXED_FILES    | 数字 | 设置注册固定描述符，当大于软上限时，会使用软上线值。                         |
| RIO_PREP_SQE_BATCH_SIZE        | 数字 | 准备 SQE 的缓冲大小，默认为 1024 的大小。                         |
| RIO_PREP_SQE_BATCH_TIME_WINDOW | 数字 | 准备 SQE 批处理时长，默认 500 纳秒。                            |
| RIO_PREP_SQE_BATCH_IDLE_TIME   | 数字 | 准备 SQE 空闲时长，默认 15 秒。                               |
| RIO_PREP_SQE_BATCH_AFF_CPU     | 数字 | 设置准备 SQE 线程所亲和的 CPU。                               |
| RIO_WAIT_CQE_BATCH_SIZE        | 数字 | 获取 CQE 的批大小，默认为 1024 的大小。                          |
| RIO_WAIT_CQE_BATCH_AFF_CPU     | 布尔 | 设置获取 CQE 线程所亲和的 CPU。                               |
| RIO_WAIT_CQE_BATCH_TIME_CURVE  | 文本 | 设置等待 CQ 策略曲线，如 `1:15s, 8:2us, 16:1ms`。             |

注意事项：
* `IOURING_SETUP_FLAGS` 与系统内核版本有关联，请务必确认版本。
* `IORING_SETUP_SQPOLL` 取决于运行环境，非常吃配置，请自行选择配置进行调试。
* `IOURING_SETUP_FLAGS_SCHEMA` 优先级低于 `IOURING_SETUP_FLAGS` 。
* `DEFAULT` 为 `IORING_SETUP_COOP_TASKRUN`
* `PERFORMANCE` 为 `IORING_SETUP_SQPOLL` 和 `IORING_SETUP_SQ_AFF`，所以非常吃配置，但是会减少系统调用。
* `RIO_IOURING_REG_FIXED_BUFFERS` 为 `rio.FixedReaderWriter` 的前置必要条件，如果使用固定读写，必须设置该变量来注册。
* `RIO_IOURING_REG_FIXED_FILES` 为 `rio.FixedReaderWriter`、`rio.FixedFd` 和 `AutoFixedFdInstall` 的前置必要条件，如果使用固定文件，必须设置该变量来注册。
* `RIO_WAIT_CQE_BATCH_TIME_CURVE` 的第一个节点的时长建议大一些，太小会引发忙等待。


