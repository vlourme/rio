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

func ListenTCP(network string, addr *net.TCPAddr) (*TCPListener, error) {
	config := ListenConfig{
		KeepAliveConfig: net.KeepAliveConfig{Enable: true},
	}
	ctx := context.Background()
	return config.ListenTCP(ctx, network, addr)
}

func (lc *ListenConfig) ListenTCP(ctx context.Context, network string, addr *net.TCPAddr) (*TCPListener, error) {
	// vortexes
	vortexes, vortexesErr := aio.New(lc.VortexesOptions...)
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

	fd, fdErr := sysLn.Listen(sys.ListenOptions{
		MultipathTCP: lc.MultipathTCP,
		FastOpen:     lc.FastOpen,
	})
	if fdErr != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: fdErr}
	}
	// ctx
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	// vortexes start
	vortexes.Start(ctx)
	// ln
	ln := &TCPListener{
		ctx:             ctx,
		cancel:          cancel,
		fd:              fd,
		vortexes:        vortexes,
		useSendZC:       lc.UseSendZC,
		keepAlive:       lc.KeepAlive,
		keepAliveConfig: lc.KeepAliveConfig,
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
	tcpConn := &TCPConn{
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

type TCPConn struct {
	connection
}

func (conn *TCPConn) SyscallConn() (syscall.RawConn, error) {
	return newRawConnection(conn.fd), nil
}

func (conn *TCPConn) ReadFrom(r io.Reader) (int64, error) {
	return 0, &net.OpError{Op: "readfrom", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: nil}
}

func (conn *TCPConn) WriteTo(w io.Writer) (int64, error) {
	return 0, &net.OpError{Op: "writeto", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: nil}
}

func (conn *TCPConn) CloseRead() error {
	if err := conn.fd.CloseRead(); err != nil {
		return &net.OpError{Op: "close", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *TCPConn) CloseWrite() error {
	if err := conn.fd.CloseWrite(); err != nil {
		return &net.OpError{Op: "close", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *TCPConn) SetLinger(sec int) error {
	if err := conn.fd.SetLinger(sec); err != nil {
		return &net.OpError{Op: "set", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *TCPConn) SetNoDelay(noDelay bool) error {
	if err := conn.fd.SetNoDelay(noDelay); err != nil {
		return &net.OpError{Op: "set", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *TCPConn) SetKeepAlive(keepalive bool) error {
	if err := conn.fd.SetKeepAlive(keepalive); err != nil {
		return &net.OpError{Op: "set", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *TCPConn) SetKeepAlivePeriod(period time.Duration) error {
	if err := conn.fd.SetKeepAlivePeriod(period); err != nil {
		return &net.OpError{Op: "set", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *TCPConn) SetKeepAliveConfig(config net.KeepAliveConfig) error {
	if err := conn.fd.SetKeepAliveConfig(config); err != nil {
		return &net.OpError{Op: "set", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *TCPConn) MultipathTCP() (bool, error) {
	ok := sys.IsUsingMultipathTCP(conn.fd)
	return ok, nil
}
