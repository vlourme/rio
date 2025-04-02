//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"net"
	"syscall"
	"time"
)

type ListenerFd struct {
	NetFd
	backlog      int
	acceptFn     func() (nfd *Conn, err error)
	acceptFuture *acceptFuture
}

func (fd *ListenerFd) init() {
	if fd.vortex.MultishotAcceptEnabled() {
		future, futureErr := newAcceptFuture(fd)
		if futureErr == nil {
			fd.acceptFuture = future
			fd.acceptFn = fd.acceptFuture.accept
		} else {
			fd.acceptFn = fd.accept
		}
	}
}

func (fd *ListenerFd) Bind(addr net.Addr) error {
	return fd.bind(addr)
}

func (fd *ListenerFd) Accept() (nfd *Conn, err error) {
	nfd, err = fd.acceptFn()
	return
}

func (fd *ListenerFd) accept() (nfd *Conn, err error) {
	deadline := fd.readDeadline
	acceptAddr := &syscall.RawSockaddrAny{}
	acceptAddrLen := syscall.SizeofSockaddrAny
	acceptAddrLenPtr := &acceptAddrLen

	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareAccept(fd, acceptAddr, acceptAddrLenPtr)
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

func (fd *ListenerFd) Close() error {
	if fd.acceptFuture != nil {
		_ = fd.acceptFuture.Cancel()
	}
	return fd.NetFd.Close()
}

func (fd *ListenerFd) newAcceptedConnFd(accepted int) (cfd *Conn) {
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
		sendZCEnabled:    fd.vortex.SendZCEnabled(),
		sendMSGZCEnabled: fd.vortex.SendMSGZCEnabled(),
	}
	if fd.Registered() {
		cfd.direct = accepted
	} else {
		cfd.regular = accepted
	}
	cfd.init()
	return
}

func newAcceptFuture(ln *ListenerFd) (future *acceptFuture, err error) {
	f := &acceptFuture{
		ln: ln,
	}
	if err = f.submit(); err == nil {
		future = f
	}
	return
}

type acceptFuture struct {
	ln      *ListenerFd
	op      *Operation
	handler *acceptOperationHandler
	timer   *time.Timer
	addr    *syscall.RawSockaddrAny
	addrLen *int
}

func (f *acceptFuture) submit() (err error) {
	f.handler = &acceptOperationHandler{
		ch: make(chan Result, f.ln.backlog),
	}

	acceptAddrLen := syscall.SizeofSockaddrAny
	f.addr = &syscall.RawSockaddrAny{}
	f.addrLen = &acceptAddrLen

	f.op = f.ln.vortex.acquireOperation()
	f.op.Hijack()
	f.op.PrepareAcceptMultishot(f.ln, f.addr, f.addrLen, f.handler)
	if ok := f.ln.vortex.Submit(f.op); !ok {
		// release op
		op := f.op
		f.op = nil
		op.Complete()
		f.ln.vortex.releaseOperation(op)
		// close handler
		_ = f.handler.Close()
		// return cancelled
		err = ErrCancelled
		return
	}
	return
}

func (f *acceptFuture) accept() (nfd *Conn, err error) {
	var (
		handler  = f.handler
		ln       = f.ln
		timer    = f.timer
		deadline = ln.readDeadline
		vortex   = f.ln.vortex
		accepted = -1
	)
	if !deadline.IsZero() {
		timer = vortex.acquireTimer(time.Until(deadline))
		defer vortex.releaseTimer(timer)
	}

	if timer == nil {
		result, ok := <-handler.ch
		if !ok {
			err = ErrCancelled
			return
		}
		accepted, err = result.N, result.Err
	} else {
		select {
		case result, ok := <-handler.ch:
			if !ok {
				err = ErrCancelled
				return
			}
			accepted, err = result.N, result.Err
			break
		case <-timer.C:
			err = ErrTimeout
			break
		}
	}

	if err != nil {
		return
	}

	nfd = ln.newAcceptedConnFd(accepted)
	return
}

func (f *acceptFuture) Cancel() (err error) {
	if f.op != nil {
		op := f.op
		f.op = nil
		err = f.ln.vortex.CancelOperation(op)
		op.Complete()
		f.ln.vortex.releaseOperation(op)
		_ = f.handler.Close()
	}
	return
}

type acceptOperationHandler struct {
	ch chan Result
}

func (h *acceptOperationHandler) Handle(n int, flags uint32, err error) {
	if err != nil {
		if errors.Is(err, syscall.ECANCELED) {
			err = ErrCancelled
		}
		h.ch <- Result{n, flags, err}
		return
	}
	if flags&liburing.IORING_CQE_F_MORE == 0 {
		h.ch <- Result{n, flags, ErrCancelled}
		return
	}
	h.ch <- Result{n, flags, nil}
	return
}

func (h *acceptOperationHandler) Close() error {
	close(h.ch)
	return nil
}
