//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"net"
	"syscall"
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
	alloc := fd.Registered()
	deadline := fd.readDeadline
	acceptAddr := &syscall.RawSockaddrAny{}
	acceptAddrLen := syscall.SizeofSockaddrAny
	acceptAddrLenPtr := &acceptAddrLen

	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).WithDirectAlloc(alloc).PrepareAccept(fd, acceptAddr, acceptAddrLenPtr)
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
	addr    *syscall.RawSockaddrAny
	addrLen *int
}

func (f *acceptFuture) submit() (err error) {
	acceptAddrLen := syscall.SizeofSockaddrAny
	f.addr = &syscall.RawSockaddrAny{}
	f.addrLen = &acceptAddrLen
	alloc := f.ln.Registered()
	f.op = NewOperation(f.ln.backlog)
	f.op.Hijack()
	f.op.WithDirectAlloc(alloc).PrepareAcceptMultishot(f.ln, f.addr, f.addrLen)
	if ok := f.ln.vortex.Submit(f.op); !ok {
		f.op.Close()
		f.op = nil
		err = ErrCanceled
	}
	return
}

func (f *acceptFuture) accept() (nfd *Conn, err error) {
	var (
		op       = f.op
		ln       = f.ln
		deadline = ln.readDeadline
		vortex   = f.ln.vortex
		accepted = -1
		cqeFlags uint32
	)
	accepted, cqeFlags, err = vortex.awaitOperationWithDeadline(op, deadline)
	if err != nil {
		return
	}
	if cqeFlags&liburing.IORING_CQE_F_MORE == 0 {
		err = ErrCanceled
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
		op.Close()
	}
	return
}
