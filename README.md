# RIO

基于`IOURING`的`AIO`网络库，非`CGO`方式，且遵循标准库使用设计模式。

支持协议：`TCP`、`UDP`、`UNIX`、`UNIXGRAM`（`IP`为代理标准库）。

`RIO` 是遵循标准库使用方式的，是可以非常方便的投入使用，所以它不是个玩具。

***Linux 内核版本需要`>= 6.8`，推荐版本为`>= 6.13`。***

## 特性
* 基于 `IOURING` 的实现
* 基于 `net.Listener` `net.Conn`  和 `net.PacketConn` 的实现
* 使用批处理方式来减少系统调用的开销
* 支持 `TLS`
* 支持 `FIXED BUFFER` 和 `FIXED FILE`
* 支持 `SEND_ZC` 和 `SENDMSG_ZC`
* 支持动态调整 `RIO_WAIT_CQE_TIME_CURVE` 控制不同场景的性能

## 注意
* WSL2中不能开启`networkingMode=mirrored`。
* 当启用`BUFFER AND RING`时，不要开启`IORING_SETUP_SINGLE_ISSUER`，如`MULTISHOT_RECV`。
* 当内核版本`< 6.13`，不能自己拨号自己，因为在同一个`RING`里对方关闭时是收不到`EOF`。


## 性能

<img src="benchmark/benchmark_tcpkali_C50T10s.png" width="336" height="144" alt="echo benchmark">
<img src="benchmark/benchmark_tcpkali_C50R5K.png" width="336" height="144" alt="echo benchmark">
<img src="benchmark/benchmark_local.png" width="336" height="144" alt="echo benchmark">

<details>
<summary>详细信息</summary>

使用 `tcpkali` 进行压力测试，[基准测试代码地址](https://github.com/brickingsoft/rio_examples/tree/main/benchmark) 。


环境：

| 端   | 平台      | IP              | OS                                           | 规格      |
|-----|---------|-----------------|----------------------------------------------|---------|
| 客户端 | WSL2    | 192.168.100.1   | Ubuntu22.04 （6.13.6-microsoft-standard-WSL2） | 4C 16G  |
| 服务端 | Hyper-V | 192.168.100.120 | Ubuntu24.10（6.11.0-8-generic）                | 4C 0.5G |


### C50 T10s
50链接10秒。

```shell
tcpkali --workers 1 -c 50 -T 10s -m "PING" 192.168.100.120:9000
```
结果：

| 种类   | 速率 （pps） | 说明       | 性能    |
|------|----------|----------|-------|
| RIO  | 23263.3  | 稳定在23000 | 100 % |
| EVIO | 18855.5  | 稳定在18000 | 81 %  |
| GNET | 18284.7  | 稳定在18000 | 79 %  |
| NET  | 14638.6  | 稳定在14000 | 63 %  |


### C50 R5k
50链接重复5000次。

```shell
tcpkali --workers 1 -c 50 -r 5k -m "PING" 192.168.100.120:9000
```
结果：

| 种类   | 速率 （pps） | 说明       | 性能    |
|------|----------|----------|-------|
| RIO  | 44031.1  | 稳定在44000 | 100 % |
| EVIO | 27997.6  | 稳定在28000 | 64 %  |
| GNET | 28817.2  | 稳定在28000 | 65 %  |
| NET  | 27874.5  | 稳定在28000 | 63 %  |


</details>


## 使用

```shell
go get -u github.com/brickingsoft/rio
```

基本使用，将 `github.com/brickingsoft/rio` 替换 `net`。
```go
// 将 net.Listen() 替换成 rio.Listen() 
ln, lnErr := rio.Listen("tcp", ":9000")
// 将 net.Dial() 替换成 rio.Dial() 
conn, dialErr := rio.Dial("tcp", "127.0.0.1:9000")
```


## 进阶使用

### TLS

使用内置`security`方式。
```go
// server("github.com/brickingsoft/rio/security")
ln, _ = security.Listen("tcp", ":9000", config)

// client("github.com/brickingsoft/rio/security")
conn, _ = security.Dial("tcp", "127.0.0.1:9000", config)
```

使用包裹方式。
```go

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

### 类型转换

```go
// 转换成 TCP 链接 
tcpConn, ok := conn.(*rio.TCPConn)
// 转换成 UDP 链接
udpConn, ok := conn.(*rio.UDPConn)
// 转换成 UNIX 链接
unixConn, ok := conn.(*rio.UnixConn)
// 转换成 RIO 链接
rioConn, ok := conn.(rio.Conn)
```


### Config <a id="config"></a>

`rio.ListenConfig` 与 `net.ListenConfig` 是类似的，通过配置来监听。
```go

config := rio.ListenConfig{
    Control:            nil,                     // 设置控制器
    KeepAlive:          0,                       // 设置 KeepAlive 时长
    KeepAliveConfig:    net.KeepAliveConfig{},   // 设置 KeepAlive 详细配置
    MultipathTCP:       false,                   // 是否多路TCP模式
    ReusePort:          false,                   // 是否重用端口（同时开启cBPF）
    SendZC:             false,                   // 是否使用 Zero-Copy 方式发送（某些场景会遥测不到但其实是发送了，如 TCPKALI）
    MultishotAccept:    false,                   // 是否单投多发模式来接受链接
    DisableDirectAlloc: false,                   // 禁止自动注册（6.7后有效）
    Vortex:             nil,                     // 自定义 iouring
}
ln, lnErr := config.Listen(context.Background(), "tcp", ":9000")
```

`rio.Dialer` 与 `net.Dialer` 是类似的，通过配置来拨号。
```go
dialer := rio.Dialer{
    Timeout:            0,                          // 超时
    Deadline:           time.Time{},                // 死期
    KeepAlive:          0,                          // 设置 KeepAlive 时长
    KeepAliveConfig:    net.KeepAliveConfig{},      // 设置 KeepAlive 详细配置
    LocalAddr:          nil,                        // 本地地址
    FallbackDelay:      0,                          // 并行回退延时   
    MultipathTCP:       false,                      // 是否多路TCP模式
    SendZC:             false,                      // 是否使用 Zero-Copy 方式发送（某些场景会遥测不到但其实是发送了，如 TCPKALI）
    DisableDirectAlloc: false,                      // 禁止自动注册（6.7后有效）
    Control:            nil,                        // 设置控制器
    ControlContext:     nil,                        // 设置带上下文的控制器
    Vortex:             nil,                        // 自定义 iouring
}
conn, dialErr := dialer.DialContext(context.Background(), "tcp", "127.0.0.1:9000")
```

### Fixed Buffer

使用注册的固定字节缓存进行读写，其效果是相比非注册模式会减少字节缓存在映射时上下文切换。

如需使用，请务必设置固定字节缓存在先，可以通过名为`RIO_IOURING_REG_FIXED_BUFFERS`的环境变量或`rio.Presets(aio.WithRegisterFixedBuffer(..))` 方式来设置。

```go
// convert
fixed, ok := conn.(rio.Conn)
// acquire buf
buf := fixed.AcquireRegisteredBuffer()
defer fixed.ReleaseRegisteredBuffer(buf)
// check buf
if buf == nil {
	// not registered or no buf remain
	// use normal read
}
// read
rn, rErr := buf.ReadFixed(buf)
// write
wn, wErr := fixed.WriteFixed(buf)
```


### 参数设置
通过设置环境变量进行调控，具体详见 [IOURING](https://man.archlinux.org/man/extra/liburing/io_uring_setup.2.en)。

| 名称                                   | 值  | 说明                                                 |
|--------------------------------------|----|----------------------------------------------------|
| RIO_IOURING_ENTRIES                  | 数字 | 环大小，默认为最大值 16384。                                  |
| RIO_IOURING_SETUP_FLAGS              | 文本 | 标识，如`IORING_SETUP_SQPOLL, IORING_SETUP_SQ_AFF`等。   |
| RIO_IOURING_SQ_THREAD_CPU            | 数字 | 设置 SQ 环锁亲和的 CPU。                                   |
| RIO_IOURING_SQ_THREAD_IDLE           | 数字 | 在含有`IORING_SETUP_SQPOLL`标识时，设置空闲时长，单位为毫秒，默认是 10 秒。 |
| RIO_IOURING_REG_FIXED_BUFFERS        | 文本 | 设置注册固定字节缓存，格式为 `单个大小, 个数`， 如`4096, 1024`。          |
| RIO_IOURING_REG_FIXED_FILES          | 数字 | 设置注册固定描述符，当内核`>=6.7`后，默认`65535`。                   |
| RIO_IOURING_REG_FIXED_FILES_RESERVED | 数字 | 设置预留的注册固定描述符，只用于非`generic`的内核，如`WSL2`。             |
| RIO_PREP_SQE_AFF_CPU                 | 数字 | 设置准备 SQE 线程所亲和的 CPU。                               |
| RIO_PREP_SQE_BATCH_MIN_SIZE          | 数字 | 设置准备 SQE 的最小批量大小。                                  |
| RIO_PREP_SQE_BATCH_TIME_WINDOW       | 文本 | 批量准备 SQE 的时间窗口。格式为时长，如`500us`。                     |
| RIO_PREP_SQE_BATCH_IDLE_TIME         | 文本 | 批量设置准备 SQE 空闲时长。格式为时长，如`15s`。                      |
| RIO_WAIT_CQE_MODE                    | 枚举 | 获取 CQE 的方式。`PUSH` 和 `PULL`。                        |
| RIO_WAIT_CQE_TIME_CURVE              | 文本 | 设置等待 CQE 策略曲线，如 `8:10us, 16:100us`。                |
| RIO_WAIT_CQE_PULL_IDLE_TIME          | 文本 | 设置拉式等待 CQE 的空闲时间。 格式为时长，如`15s`。                    |

注意事项：
* `RIO_IOURING_SETUP_FLAGS` 与系统内核版本有关联，请务必确认版本，但程序会自动过滤掉与版本不符的标识，但不解决标识冲突。
  * 默认通过 CPU 数量决定标识，小于4核为`IORING_SETUP_COOP_TASKRUN | IORING_SETUP_DEFER_TASKRUN`，反之为 `IORING_SETUP_SQPOLL`。
  * `IORING_SETUP_SQPOLL` 取决于运行环境，非常吃配置，请自行选择配置进行调试。
  * `IORING_SETUP_SQ_AFF` 激活时，且是容器环境，此时需要注意 CPU 的相关设置。
* `RIO_IOURING_REG_FIXED_BUFFERS` 为 使用注册缓存区读写的前置必要条件，如果使用固定读写，必须设置该变量来注册。
* `RIO_IOURING_REG_FIXED_FILES` 
  * 版本低于`6.0`时不可用。
  * 版本低于`6.7`时只支持手动注册，默认是 1024。
  * 版本高于等于`6.7`时支持自动注册，默认是 65535，`RIO_IOURING_REG_FIXED_FILES_RESERVED` 是给手动注册预留的数量，默认8。
* `RIO_WAIT_CQE_MODE` 
  * `PUSH` 为内核推送可收割通知的方式，当 `IORING_SETUP_SQPOLL` 时，只能用 `PUSH`。
  * `PULL` 为用户态主动从内核拉可收割的完成事件。
* `RIO_WAIT_CQE_TIME_CURVE` 是用于调整批处理性能，花费多少时间等待多少个进行批处理。


