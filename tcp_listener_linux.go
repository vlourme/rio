//go:build linux

package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/sys"
	"net"
	"syscall"
)

func ListenTCP(network string, addr *net.TCPAddr, options ...Option) (*TCPListener, error) {
	opts := Options{}
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
		ctx:      ctx,
		cancel:   cancel,
		fd:       fd,
		vortexes: vortexes,
	}
	return ln, nil
}

type TCPListener struct {
	ctx      context.Context
	cancel   context.CancelFunc
	fd       *sys.Fd
	vortexes *aio.Vortexes
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

	// todo setup keep alive
	// conn
	side := vortexes.Side()
	conn = newTcpConnection(ctx, side, cfd)
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
