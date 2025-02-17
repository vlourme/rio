//go:build linux

package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/kernel"
	"github.com/brickingsoft/rio/pkg/sys"
	"io"
	"net"
	"sync/atomic"
	"syscall"
	"time"
)

type tcpConnection struct {
	connection
}

func (conn *tcpConnection) SyscallConn() (syscall.RawConn, error) {
	return newRawConnection(conn.fd), nil
}

func (conn *tcpConnection) ReadFrom(r io.Reader) (int64, error) {
	return 0, &net.OpError{Op: "readfrom", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: nil}
}

func (conn *tcpConnection) WriteTo(w io.Writer) (int64, error) {
	return 0, &net.OpError{Op: "writeto", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: nil}
}

func (conn *tcpConnection) CloseRead() error {
	if err := conn.fd.CloseRead(); err != nil {
		return &net.OpError{Op: "close", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *tcpConnection) CloseWrite() error {
	if err := conn.fd.CloseWrite(); err != nil {
		return &net.OpError{Op: "close", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *tcpConnection) SetLinger(sec int) error {
	if err := conn.fd.SetLinger(sec); err != nil {
		return &net.OpError{Op: "set", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *tcpConnection) SetNoDelay(noDelay bool) error {
	if err := conn.fd.SetNoDelay(noDelay); err != nil {
		return &net.OpError{Op: "set", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *tcpConnection) SetKeepAlive(keepalive bool) error {
	if err := conn.fd.SetKeepAlive(keepalive); err != nil {
		return &net.OpError{Op: "set", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *tcpConnection) SetKeepAlivePeriod(period time.Duration) error {
	if err := conn.fd.SetKeepAlivePeriod(period); err != nil {
		return &net.OpError{Op: "set", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *tcpConnection) SetKeepAliveConfig(config net.KeepAliveConfig) error {
	if err := conn.fd.SetKeepAliveConfig(config); err != nil {
		return &net.OpError{Op: "set", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *tcpConnection) MultipathTCP() (bool, error) {
	ok := sys.IsUsingMultipathTCP(conn.fd)
	return ok, nil
}

func ListenTCP(network string, addr *net.TCPAddr, options ...ListenOption) (*TCPListener, error) {
	opts := ListenOptions{}
	for _, o := range options {
		if err := o(&opts); err != nil {
			return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: err}
		}
	}
	// vortexes
	vortexesOptions := opts.VortexesOptions
	if vortexesOptions == nil {
		vortexesOptions = make([]aio.Option, 0)
	}
	vortexes, vortexesErr := aio.New(vortexesOptions...)
	if vortexesErr != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: vortexesErr}
	}

	// fd
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: net.UnknownNetworkError(network)}
	}
	if addr == nil {
		addr = &net.TCPAddr{}
	}
	sysLn, sysErr := sys.NewListener(network, addr.String())
	if sysErr != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: sysErr}
	}
	lnOpts := make([]sys.ListenOption, 0, 1)
	if opts.MultipathTCP {
		lnOpts = append(lnOpts, sys.UseMultipath())
	}
	if n := opts.FastOpen; n > 0 {
		lnOpts = append(lnOpts, sys.UseFastOpen(n))
	}
	fd, fdErr := sysLn.Listen(lnOpts...)
	if fdErr != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: fdErr}
	}
	// ctx
	rootCtx := opts.Ctx
	if rootCtx == nil {
		rootCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(rootCtx)
	// vortexes start
	vortexes.Start(ctx)
	// ln
	ln := &TCPListener{
		ctx:             ctx,
		cancel:          cancel,
		fd:              fd,
		vortexes:        vortexes,
		useSendZC:       opts.UseSendZC,
		keepAlive:       opts.KeepAlive,
		keepAliveConfig: opts.KeepAliveConfig,
	}
	ln.checkUseSendZC()
	return ln, nil
}

type TCPListener struct {
	ctx             context.Context
	cancel          context.CancelFunc
	fd              *sys.Fd
	vortexes        *aio.Vortexes
	useSendZC       bool
	keepAlive       time.Duration
	keepAliveConfig net.KeepAliveConfig
}

func (ln *TCPListener) Accept() (conn net.Conn, err error) {
	ctx := ln.ctx
	fd := ln.fd.Socket()
	vortexes := ln.vortexes
	center := vortexes.Center()
	addr := &syscall.RawSockaddrAny{}
	addrLen := syscall.SizeofSockaddrAny
	future := center.PrepareAccept(ctx, fd, addr, addrLen)
	accepted, acceptErr := future.Await(ctx)
	if acceptErr != nil {
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: acceptErr}
		return
	}

	cfd := sys.NewFd(ln.fd.Net(), accepted, ln.fd.Family(), ln.fd.SocketType())
	// local addr
	if err = cfd.LoadLocalAddr(); err != nil {
		_ = cfd.Close()
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
		return
	}
	// remote addr
	sa, saErr := sys.RawSockaddrAnyToSockaddr(addr)
	if saErr != nil {
		if err = cfd.LoadRemoteAddr(); err != nil {
			_ = cfd.Close()
			err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
			return
		}
	}
	localAddr := sys.SockaddrToAddr(ln.fd.Net(), sa)
	cfd.SetRemoteAddr(localAddr)

	// tcp conn
	side := vortexes.Side()
	cc, cancel := context.WithCancel(ctx)
	tcpConn := &tcpConnection{
		connection{
			ctx:          cc,
			cancel:       cancel,
			fd:           cfd,
			useZC:        ln.useSendZC,
			vortex:       side,
			readTimeout:  atomic.Int64{},
			writeTimeout: atomic.Int64{},
		},
	}
	// no delay
	_ = tcpConn.SetNoDelay(true)
	// keepalive
	keepAliveConfig := ln.keepAliveConfig
	if !keepAliveConfig.Enable && ln.keepAlive >= 0 {
		keepAliveConfig = net.KeepAliveConfig{
			Enable: true,
			Idle:   ln.keepAlive,
		}
	}
	if keepAliveConfig.Enable {
		_ = tcpConn.SetKeepAliveConfig(keepAliveConfig)
	}
	// conn
	conn = tcpConn
	return
}

func (ln *TCPListener) Close() error {
	defer ln.cancel()
	defer func(vortexes *aio.Vortexes) {
		_ = vortexes.Close()
	}(ln.vortexes)
	if err := ln.fd.Close(); err != nil {
		return &net.OpError{Op: "close", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
	}
	return nil
}

func (ln *TCPListener) Addr() net.Addr {
	return ln.fd.LocalAddr()
}

func (ln *TCPListener) checkUseSendZC() {
	if ln.useSendZC {
		ver, verErr := kernel.GetKernelVersion()
		if verErr != nil {
			ln.useSendZC = false
			return
		}
		target := kernel.Version{
			Kernel: ver.Kernel,
			Major:  6,
			Minor:  0,
			Flavor: ver.Flavor,
		}
		if kernel.CompareKernelVersion(*ver, target) < 0 {
			ln.useSendZC = false
		}
	}
}
