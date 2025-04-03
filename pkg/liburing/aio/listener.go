//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"net"
	"sync"
	"syscall"
	"time"
)

type Listener struct {
	NetFd
	backlog  int
	acceptFn func() (nfd *Conn, err error)
	handler  *AcceptMultishotHandler
}

func (fd *Listener) init() {
	if fd.vortex.multishotAcceptEnabled() {
		handler, handlerErr := newAcceptMultishotHandler(fd)
		if handlerErr == nil {
			fd.handler = handler
			fd.acceptFn = fd.handler.Accept
		} else {
			fd.acceptFn = fd.accept
		}
	}
}

func (fd *Listener) Bind(addr net.Addr) error {
	return fd.bind(addr)
}

func (fd *Listener) Accept() (nfd *Conn, err error) {
	nfd, err = fd.acceptFn()
	return
}

func (fd *Listener) accept() (nfd *Conn, err error) {
	deadline := fd.readDeadline
	acceptAddr := &syscall.RawSockaddrAny{}
	acceptAddrLen := syscall.SizeofSockaddrAny
	param := &prepareAcceptParam{
		addr:    acceptAddr,
		addrLen: &acceptAddrLen,
	}

	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareAccept(fd, param)
	accepted, _, acceptErr := fd.vortex.submitAndWait(op)
	fd.vortex.releaseOperation(op)
	if acceptErr != nil {
		err = acceptErr
		return
	}

	nfd = fd.newAcceptedConnFd(accepted)

	sa, saErr := sys.RawSockaddrAnyToSockaddr(acceptAddr)
	if saErr == nil {
		addr := sys.SockaddrToAddr(nfd.net, sa)
		nfd.SetRemoteAddr(addr)
	}
	return
}

func (fd *Listener) Close() error {
	if fd.handler != nil {
		_ = fd.handler.Close()
	}
	return fd.NetFd.Close()
}

func (fd *Listener) newAcceptedConnFd(accepted int) (cfd *Conn) {
	cfd = &Conn{
		NetFd: NetFd{
			Fd: Fd{
				regular:       -1,
				direct:        -1,
				isStream:      fd.isStream,
				zeroReadIsEOF: fd.zeroReadIsEOF,
				vortex:        fd.vortex,
			},

			family: fd.family,
			sotype: fd.sotype,
			net:    fd.net,
			laddr:  nil,
			raddr:  nil,
		},
		sendZCEnabled:    fd.vortex.sendZCEnabled,
		sendMSGZCEnabled: fd.vortex.sendMSGZCEnabled,
	}
	if fd.Registered() {
		cfd.direct = accepted
	} else {
		cfd.regular = accepted
	}
	cfd.init()
	return
}

func newAcceptMultishotHandler(ln *Listener) (handler *AcceptMultishotHandler, err error) {
	ch := make(chan Result, ln.backlog)
	// param
	acceptAddrLen := syscall.SizeofSockaddrAny
	param := &prepareAcceptParam{
		addr:    &syscall.RawSockaddrAny{},
		addrLen: &acceptAddrLen,
	}
	// op
	op := ln.vortex.acquireOperation()
	op.Hijack()
	// handler
	handler = &AcceptMultishotHandler{
		ln:     ln,
		op:     op,
		param:  param,
		locker: sync.Mutex{},
		err:    nil,
		ch:     ch,
	}
	// prepare
	op.PrepareAcceptMultishot(ln, param, handler)
	// submit
	if err = handler.submit(); err != nil {
		op.Complete()
		ln.vortex.releaseOperation(op)
	}
	return
}

type AcceptMultishotHandler struct {
	ln     *Listener
	op     *Operation
	param  *prepareAcceptParam
	locker sync.Mutex
	err    error
	ch     chan Result
}

func (handler *AcceptMultishotHandler) Handle(n int, flags uint32, err error) {
	if err != nil {
		if errors.Is(err, syscall.ECANCELED) {
			err = ErrCanceled
		}
		handler.ch <- Result{n, flags, err}
		return
	}
	if flags&liburing.IORING_CQE_F_MORE == 0 {
		handler.ch <- Result{n, flags, ErrCanceled}
		return
	}
	handler.ch <- Result{n, flags, nil}
	return
}

func (handler *AcceptMultishotHandler) Accept() (conn *Conn, err error) {
	handler.locker.Lock()
	if handler.err != nil {
		err = handler.err
		handler.locker.Unlock()
		return
	}
	handler.locker.Unlock()
	var (
		ln       = handler.ln
		deadline = ln.readDeadline
		vortex   = handler.ln.vortex
		timer    *time.Timer
		accepted = -1
	)
	// deadline
	if !deadline.IsZero() {
		timer = vortex.acquireTimer(time.Until(deadline))
		defer vortex.releaseTimer(timer)
	}
	// read ch
RETRY:
	if timer == nil {
		result, ok := <-handler.ch
		if ok {
			accepted, err = result.N, result.Err
		} else {
			err = ErrCanceled
		}
	} else {
		select {
		case result, ok := <-handler.ch:
			if ok {
				accepted, err = result.N, result.Err
			} else {
				err = ErrCanceled
			}
			break
		case <-timer.C:
			err = ErrTimeout
			break
		}
	}
	// handle err
	if err != nil {
		if errors.Is(err, ErrIOURingSQBusy) {
			if err = handler.submit(); err != nil {
				return
			}
			goto RETRY
		}
		if errors.Is(err, ErrCanceled) {
			// set err
			handler.locker.Lock()
			handler.err = err
			if handler.op != nil {
				// release op
				op := handler.op
				handler.op = nil
				op.Complete()
				handler.ln.vortex.releaseOperation(op)
			}
			handler.locker.Unlock()
		}
		return
	}
	// new conn
	conn = ln.newAcceptedConnFd(accepted)
	return
}

func (handler *AcceptMultishotHandler) Close() (err error) {
	handler.locker.Lock()
	defer handler.locker.Unlock()
	if handler.op == nil {
		return
	}
	op := handler.op
	if err = handler.ln.vortex.cancelOperation(op); err != nil {
		// use cancel fd when cancel op failed
		handler.ln.Cancel()
		// reset err when fd was canceled
		err = nil
	}

	op.Complete()
	handler.ln.vortex.releaseOperation(op)
	handler.op = nil

	return
}

func (handler *AcceptMultishotHandler) submit() (err error) {
	if ok := handler.ln.vortex.submit(handler.op); !ok {
		err = ErrCanceled
		return
	}
	return
}
