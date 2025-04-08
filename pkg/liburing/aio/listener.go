//go:build linux

package aio

import (
	"errors"
	"fmt"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
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
	if fd.multishot {
		handler, handlerErr := newAcceptMultishotHandler(fd)
		if handlerErr == nil {
			fd.handler = handler
			fd.acceptFn = fd.handler.Accept
		} else {
			fd.acceptFn = fd.accept
		}
	}
}

func (fd *Listener) Accept() (nfd *Conn, err error) {
	nfd, err = fd.acceptFn()
	return
}

func (fd *Listener) accept() (nfd *Conn, err error) {
	acceptAddr := &syscall.RawSockaddrAny{}
	acceptAddrLen := syscall.SizeofSockaddrAny
	param := &prepareAcceptParam{
		addr:    acceptAddr,
		addrLen: &acceptAddrLen,
	}

	op := fd.eventLoop.resource.AcquireOperation()
	op.WithDeadline(fd.eventLoop.resource, fd.readDeadline).PrepareAccept(fd, param)
	accepted, _, acceptErr := fd.eventLoop.SubmitAndWait(op)
	fd.eventLoop.resource.ReleaseOperation(op)

	if acceptErr != nil {
		err = acceptErr
		return
	}
	// dispatch to worker
	op = fd.eventLoop.resource.AcquireOperation()
	if dispatchErr := fd.eventLoop.group.Dispatch(accepted, op); dispatchErr != nil {
		fd.eventLoop.resource.ReleaseOperation(op)
		cfd := &Fd{direct: accepted}
		_ = cfd.Close()
		err = dispatchErr
		return
	}
	if _, _, err = op.Await(); err != nil {
		fd.eventLoop.resource.ReleaseOperation(op)
		return
	}
	worker := (*EventLoop)(op.addr)
	fd.eventLoop.resource.ReleaseOperation(op)

	nfd = fd.newAcceptedConnFd(accepted, worker)

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

func (fd *Listener) newAcceptedConnFd(accepted int, event *EventLoop) (cfd *Conn) {
	cfd = &Conn{
		NetFd: NetFd{
			Fd: Fd{
				regular:       -1,
				direct:        accepted,
				isStream:      fd.isStream,
				zeroReadIsEOF: fd.zeroReadIsEOF,
				readDeadline:  time.Time{},
				writeDeadline: time.Time{},
				multishot:     fd.multishot,
				eventLoop:     event,
			},
			family:           fd.family,
			sotype:           fd.sotype,
			net:              fd.net,
			laddr:            nil,
			raddr:            nil,
			sendZCEnabled:    fd.sendZCEnabled,
			sendMSGZCEnabled: fd.sendZCEnabled,
		},
		recvFn:  nil,
		handler: nil,
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
	op := ln.eventLoop.resource.AcquireOperation()
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
		ln.eventLoop.resource.ReleaseOperation(op)
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
		// todo resubmit or done
		handler.ch <- Result{n, flags, ErrCanceled}
		return
	}
	// todo send to dispatch ch, dispatch handler to dispatch and send to handler.ch
	// handler.ch is {conn, err}, not a result
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
		timer    *time.Timer
		accepted = -1
	)
	// deadline
	if !deadline.IsZero() {
		timer = handler.ln.eventLoop.resource.AcquireTimer(time.Until(deadline))
		defer handler.ln.eventLoop.resource.ReleaseTimer(timer)
	}
	// read ch
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
		if errors.Is(err, ErrCanceled) {
			// set err
			handler.locker.Lock()
			handler.err = err
			handler.locker.Unlock()
		}
		return
	}
	// dispatch to worker
	fmt.Println("dispatch")
	op := handler.ln.eventLoop.resource.AcquireOperation()
	if dispatchErr := handler.ln.eventLoop.group.Dispatch(accepted, op); dispatchErr != nil {
		fmt.Println("dispatch", dispatchErr)
		handler.ln.eventLoop.resource.ReleaseOperation(op)
		cfd := &Fd{direct: accepted}
		_ = cfd.Close()
		err = dispatchErr
		return
	}
	if _, _, err = op.Await(); err != nil {
		fmt.Println("dispatch wait", err)
		handler.ln.eventLoop.resource.ReleaseOperation(op)
		return
	}
	worker := (*EventLoop)(op.addr)
	handler.ln.eventLoop.resource.ReleaseOperation(op)
	fmt.Println("dispatch wait", accepted, worker.Fd())

	// new conn
	conn = ln.newAcceptedConnFd(accepted, worker)
	return
}

func (handler *AcceptMultishotHandler) Close() (err error) {
	handler.locker.Lock()
	defer handler.locker.Unlock()
	if handler.op == nil {
		return
	}
	op := handler.op
	if err = handler.ln.eventLoop.Cancel(op); err != nil {
		if !errors.Is(handler.err, ErrCanceled) {
			// use cancel fd when cancel op failed
			handler.ln.Cancel()
		}
		// reset err when fd was canceled
		err = nil
	}

	op.Complete()
	handler.ln.eventLoop.resource.ReleaseOperation(op)
	handler.op = nil
	return
}

func (handler *AcceptMultishotHandler) submit() (err error) {
	err = handler.ln.eventLoop.Submit(handler.op)
	return
}
