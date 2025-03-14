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

func (ln *TCPListener) prepareEPollAccepting() (err error) {
	poll, pollErr := sys.OpenEPoll()
	if pollErr != nil {
		err = pollErr
		return
	}
	ln.epoll = poll
	backlog := sys.MaxListenerBacklog()
	if backlog < 1024 {
		backlog = 1024
	}
	acceptFuture := &epollAcceptFuture{
		ch:   make(chan int, backlog),
		poll: poll,
	}
	ln.acceptFuture = acceptFuture
	go func(poll *sys.EPoll, future *epollAcceptFuture) {
		_ = poll.Wait(func(fd int, events uint32) {
			future.ch <- fd
		})
	}(poll, acceptFuture)
	go func(ctx context.Context, poll *sys.EPoll) {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(1 * time.Second)
				_ = poll.Wakeup()
			}
		}
	}(ln.ctx, poll)
	poll.AddRead(ln.fd.Socket())
	return
}

func (ln *TCPListener) acceptTCPEPoll() (tc *TCPConn, err error) {
	ctx := ln.ctx
	accepted, _, waitErr := ln.acceptFuture.Await(ctx)
	if waitErr != nil {
		if errors.Is(waitErr, context.Canceled) {
			waitErr = net.ErrClosed
		}
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: waitErr}
		return
	}
	for {
		nfd, sa, acceptErr := syscall.Accept(accepted)
		if acceptErr != nil {
			if errors.Is(acceptErr, syscall.EAGAIN) {
				continue
			}
			err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: acceptErr}
			return
		}
		if err = syscall.SetNonblock(nfd, true); err != nil {
			_ = syscall.Close(nfd)
			err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
			return
		}
		// fd
		cfd := sys.NewFd(ln.fd.Net(), nfd, ln.fd.Family(), ln.fd.SocketType())
		// local addr
		if err = cfd.LoadLocalAddr(); err != nil {
			_ = cfd.Close()
			err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
			return
		}
		// remote addr
		remoteAddr := sys.SockaddrToAddr(ln.fd.Net(), sa)
		cfd.SetRemoteAddr(remoteAddr)

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
		break
	}
	return
}
