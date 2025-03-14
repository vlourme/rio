//go:build linux

package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/sys"
	"net"
	"syscall"
	"time"
)

func (ln *TCPListener) prepareMultishotAccepting() (err error) {
	vortex := ln.vortex

	fd := ln.fd.Socket()
	addr := &syscall.RawSockaddrAny{}
	addrLen := syscall.SizeofSockaddrAny
	backlog := sys.MaxListenerBacklog()
	if backlog < 1024 {
		backlog = 1024
	}
	future := vortex.PrepareAcceptMultishot(fd, addr, addrLen, backlog)
	ln.acceptFuture = &future
	return
}

func (ln *TCPListener) acceptTCPMultishot() (tc *TCPConn, err error) {
	ctx := ln.ctx

	accepted, _, acceptErr := ln.acceptFuture.Await(ctx)
	if acceptErr != nil {
		if errors.Is(acceptErr, context.Canceled) {
			acceptErr = net.ErrClosed
		}
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: acceptErr}
		return
	}
	// fd
	cfd := sys.NewFd(ln.fd.Net(), accepted, ln.fd.Family(), ln.fd.SocketType())
	// local addr
	if err = cfd.LoadLocalAddr(); err != nil {
		_ = cfd.Close()
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
		return
	}
	// remote addr
	if err = cfd.LoadRemoteAddr(); err != nil {
		_ = cfd.Close()
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
		return
	}

	// tcp conn
	cc, cancel := context.WithCancel(ctx)
	tc = &TCPConn{
		conn{
			ctx:           cc,
			cancel:        cancel,
			fd:            cfd,
			vortex:        ln.vortex,
			readDeadline:  time.Time{},
			writeDeadline: time.Time{},
			pinned:        false,
			useSendZC:     ln.useSendZC,
		},
	}
	// no delay
	_ = tc.SetNoDelay(true)
	// keepalive
	keepAliveConfig := ln.keepAliveConfig
	if !keepAliveConfig.Enable && ln.keepAlive >= 0 {
		keepAliveConfig = net.KeepAliveConfig{
			Enable: true,
			Idle:   ln.keepAlive,
		}
	}
	if keepAliveConfig.Enable {
		_ = tc.SetKeepAliveConfig(keepAliveConfig)
	}
	return
}
