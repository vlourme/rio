//go:build linux

package rio

import (
	"context"
	"fmt"
	"github.com/brickingsoft/errors"
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
	// start
	ctx, cancel := context.WithCancel(exec.Context())
	r.Start(ctx)

	ln := &TCPListener{
		ctx:      ctx,
		cancel:   cancel,
		fd:       fd,
		acceptCh: make(chan ring.Result, 1),
		exec:     exec,
		ring:     r,
	}
	return ln, nil
}

type TCPListener struct {
	ctx      context.Context
	cancel   context.CancelFunc
	fd       *sys.Fd
	acceptCh chan ring.Result
	exec     rxp.Executors
	ring     *ring.Ring
}

func (ln *TCPListener) Accept() (net.Conn, error) {
	r := ln.ring
	op := ring.PrepareAccept(ln.fd.Socket(), ln.acceptCh)
	if pushed := r.Push(op); !pushed {
		return nil, errors.New("busy") // todo make err
	}
	sock, err := op.Await()
	if err != nil {
		return nil, err
	}
	fmt.Println(sock)
	return nil, nil
}

func (ln *TCPListener) Close() error {
	ln.cancel()
	if err := ln.fd.Close(); err != nil {
		return &net.OpError{Op: "close", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
	}
	return nil
}

func (ln *TCPListener) Addr() net.Addr {
	return ln.fd.LocalAddr()
}
