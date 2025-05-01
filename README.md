# RIO ([中文](https://github.com/brickingsoft/rio/tree/main/README_zh.md))

An `AIO` network library based on `IOURING`, without using `CGO`, and following the design pattern of the standard library.

Supported protocols: `TCP`, `UDP`, `UNIX`, `UNIXGRAM` (`IP` is the proxy standard library).

`RIO` is a library that follows the usage pattern of the standard library and can be put into use very conveniently. Therefore, it is not a toy and can replace `NET` at a very low cost.

## NOTE
* Linux kernel version must be `>= 6.13`.
* Scenarios that only use `Dial` require `PIN` and `UNPIN` to pin the kernel thread of `IOURING`.
* `NetworkingMode=mirrored` cannot be enabled in `WSL2`.
* Since `DIRECT FD` does not support `CLOEXEC`, it is necessary to close all `FD` when the program exits (close all links when both net.Http and fasthttp implement closure).


## Features
* Based on the implementation of `net.Listener`, `net.Conn` and `net.PacketConn`.
* Use `BATCH` to reduce the overhead of `SYSTEM CALL`.
* Support `TLS`.
* Support `MULTISHOT_ACCEPT` `MULTISHOT_RECV` and `MULTISHOT_RECV_FROM`.
* Support `SEND_ZC` and `SENDMSG_ZC`.
* Support `NAPI`.
* Support `PERSIONALITY`.
* Supports `CURVE` to dynamically adjust the timeout of `WAIT CQE` to fit different scenarios.


## [Performances](https://github.com/brickingsoft/rio_examples/tree/main/benchmark) 

***TCP*** 

<img src="benchmark/benchmark_tcpkali_C50T10s.png" width="336" height="144" alt="echo benchmark">
<img src="benchmark/benchmark_tcpkali_C50R5K.png" width="336" height="144" alt="echo benchmark">

***HTTP***

<img src="benchmark/benchmark_k6.png" width="336" height="144" alt="echo benchmark">


| Endpoint | Platform | IP              | OS                                           | SKU     |
|----------|----------|-----------------|----------------------------------------------|---------|
| Client   | WSL2     | 192.168.100.1   | Ubuntu22.04 (6.13.6-microsoft-standard-WSL2) | 4C 16G  |
| Server   | Hyper-V  | 192.168.100.120 | Ubuntu24.10 (6.13.12-061312-generic)         | 4C 0.5G |


***Syscall***

![syscall_rio_sqpoll.png](benchmark/syscall_rio_sqpoll.png)

![syscall_rio_single.png](benchmark/syscall_rio_single.png)

![syscall_net.png](benchmark/syscall_net.png)

| Lib | Proportion | Desc                                                          |
|-----|------------|---------------------------------------------------------------|
| RIO | 33% (3%)   | 33% is the single publisher mode, and 3% is the SQ-POLL mode. |
| NET | 74%        | Reading, writing, Epoll, etc. account for a total of 74%.     | 


## Usage

```shell
go get -u github.com/brickingsoft/rio
```

For basic use, replace `net` with `github.com/brickingsoft/rio`.
```go
// replace net.Listen() with rio.Listen() 
ln, lnErr := rio.Listen("tcp", ":9000")
// replace net.Dial() with rio.Dial() 
conn, dialErr := rio.Dial("tcp", "127.0.0.1:9000")
```

### TLS

Use the built-in `security` approach.
```go
// server("github.com/brickingsoft/rio/security")
ln, _ = security.Listen("tcp", ":9000", config)

// client("github.com/brickingsoft/rio/security")
conn, _ = security.Dial("tcp", "127.0.0.1:9000", config)
```

### HTTP

```go
rio.Preset(
    aio.WithNAPIBusyPollTimeout(time.Microsecond * 50),
)

ln, lnErr := rio.Listen("tcp", ":9000")
if lnErr != nil {
    panic(lnErr)
    return
}

srv := &http.Server{
    Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("hello world"))
    }),
}

done := make(chan struct{}, 1)
go func(ln net.Listener, srv *http.Server, done chan<- struct{}) {
    if srvErr := srv.Serve(ln); srvErr != nil {
        if errors.Is(srvErr, http.ErrServerClosed) {
            close(done)
            return
        }
        panic(srvErr)
        return
	}
	close(done)
}(ln, srv, done)

signalCh := make(chan os.Signal, 1)
signal.Notify(signalCh, syscall.SIGINT, syscall.SIGKILL, syscall.SIGQUIT, syscall.SIGABRT, syscall.SIGTERM)
<-signalCh

if shutdownErr := srv.Shutdown(context.Background()); shutdownErr != nil {
    panic(shutdownErr)
}
<-done
```

### Types

```go
tcpConn, ok := conn.(*rio.TCPConn)
udpConn, ok := conn.(*rio.UDPConn)
unixConn, ok := conn.(*rio.UnixConn)
rioConn, ok := conn.(rio.Conn)
```

### Config

`rio.ListenConfig` is similar to `net.ListenConfig` and listens by configuration.

```go
config := rio.ListenConfig{
    Control:            nil,                    
    KeepAlive:          0,                       
    KeepAliveConfig:    net.KeepAliveConfig{},   
    MultipathTCP:       false,                   
    ReusePort:          false,                  
}
ln, lnErr := config.Listen(context.Background(), "tcp", ":9000")
```

`rio.Dialer` is similar to `net.Dialer` and dials by configuration.

```go
dialer := rio.Dialer{
    Timeout:            0,                          
    Deadline:           time.Time{},                
    KeepAlive:          0,                          
    KeepAliveConfig:    net.KeepAliveConfig{},      
    LocalAddr:          nil,                        
    FallbackDelay:      0,                           
    MultipathTCP:       false,                      
    Control:            nil,                       
    ControlContext:     nil,                        
}
conn, dialErr := dialer.DialContext(context.Background(), "tcp", "127.0.0.1:9000")
```

### PIN and UNPIN

Because `IOURING` has a resource handling step in its setup and shutdown process, and its lifecycle is tied to the user's maximum lifecycle.

To prevent an instance from shutting down when it shouldn't, its lifecycle can be manually controlled via `PIN` and `UNPIN`, generally for scenarios where there is only `DIAL` or where there is more than one `LISTEN`.

```go
// Calling before presetting and launching a link
rio.Pin()
// Called after all links are closed
rio.Unpin()
```

### CQE Wait Timeout Curve

Predefined `aio.NCurve` `aio.SCurve` and `aio.LCurve`.

| Name   | Desc  | Scenes                                   |  
|--------|-------|------------------------------------------|
| NCurve | Nil   | For non-single publishers only           | 
| SCurve | Short | For short links under single publisher   | 
| LCurve | Long  | For long links under multiple publishers | 


### Preset

Customize `IOURING` with presets.

```go
rio.Peset(
    // Set the size of the IOURING, default is 16384, maximum is 32768.
    aio.WithEntries(liburing.DefaultEntries),
    // Set the Flags of the IOURING.
    // Optimized for single threading by default, how you need to turn on SQPOLL can be set.
    aio.WithFlags(liburing.IORING_SETUP_SINGLE_ISSUER),
    // Whether to enable SEND ZERO COPY.
    // Not turned on by default. Note: Some pressure testing tools cannot detect the return value.
    aio.WithSendZCEnabled(false),
    // Whether to disable multishot mode.
    // Not disabled by default.
    // Multishots can significantly reduce SQE casts, but will require additional resources such as registering and deregistering BufferAndRing.
    // Disable multishot mode is typically used in conjunction with enabling SQPOLL to significantly reduce the overhead of SYSCALL.
    aio.WithMultishotDisabled(false),
    // Set BufferAndRing config.
    // Effective in non-prohibited multishot mode.
    // A BufferAndRing serves only one Fd.
    // The parameter size is the size of the buffer, which is recommended to be a page size.
    // The parameter count is the number of buffer nodes in the ring.
    // The parameter idle timeout is the amount of time to idle before logging out when it is no longer in use.
    aio.WithBufferAndRingConfig(4096, 32, 5*time.Second),
    // Set the CQE wait timeout curve.
    aio.WithWaitCQETimeoutCurve(aio.SCurve),
    // Set the NAPI.
    // The minimum unit of timeout time is microsecond, which is not turned on by default.
    aio.WithNAPIBusyPollTimeout(50*time.Microsecond),
)
```



