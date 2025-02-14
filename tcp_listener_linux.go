//go:build linux

package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/ring"
	"github.com/brickingsoft/rio/pkg/sys"
	"github.com/brickingsoft/rxp"
	"net"
)

func ListenTCP(network string, addr *net.TCPAddr, options ...Option) (*TCPListener, error) {
	opts := Options{}
	for _, o := range options {
		if err := o(&opts); err != nil {
			return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: err}
		}
	}
	// ring
	r, rErr := ring.New(0)
	if rErr != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: rErr}
	}
	// executors
	var exec rxp.Executors
	if exec = opts.Executors; exec == nil {
		nExec, execErr := rxp.New()
		if execErr != nil {
			return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: execErr}
		}
		exec = nExec
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
	ctx, cancel := context.WithCancel(exec.Context())
	// start iouring
	r.Start(ctx)
	// ln
	ln := &TCPListener{
		ctx:    ctx,
		cancel: cancel,
		fd:     fd,
		exec:   exec,
		ring:   r,
	}
	return ln, nil
}

type TCPListener struct {
	ctx    context.Context
	cancel context.CancelFunc
	fd     *sys.Fd
	exec   rxp.Executors
	ring   *ring.Ring
}

func (ln *TCPListener) Accept() (conn net.Conn, err error) {
	r := ln.ring
	op := r.AcquireOperation()
	fd := ln.fd.Socket()
	op.PrepareAccept(fd)
	if pushErr := r.Push(op); pushErr != nil {
		op.Discard()
		r.ReleaseOperation(op)
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: pushErr} // todo make err
		return
	}
	ctx := ln.ctx
	sock, waitErr := op.Await(ctx)
	r.ReleaseOperation(op)
	if waitErr != nil {
		err = waitErr // todo make err
		return
	}
	cfd := sys.NewFd(ln.fd.Net(), sock, ln.fd.Family(), ln.fd.SocketType())
	if err = cfd.LoadLocalAddr(); err != nil {
		_ = cfd.Close()
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
		return
	}
	if err = cfd.LoadRemoteAddr(); err != nil {
		_ = cfd.Close()
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
		return
	}
	conn = newTcpConnection(ctx, ln.ring, ln.exec, cfd)
	return
}

func (ln *TCPListener) Close() error {
	defer ln.cancel()
	defer ln.ring.Stop()
	if err := ln.fd.Close(); err != nil {
		return &net.OpError{Op: "close", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
	}
	return nil
}

func (ln *TCPListener) Addr() net.Addr {
	return ln.fd.LocalAddr()
}
