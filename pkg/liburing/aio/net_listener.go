//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"sync"
	"syscall"
)

type ListenerFd struct {
	NetFd
	acceptFuture *AcceptFuture
}

func (fd *ListenerFd) init() {
	if fd.vortex.MultishotAcceptEnabled() {
		acceptAddr := &syscall.RawSockaddrAny{}
		acceptAddrLen := syscall.SizeofSockaddrAny
		acceptAddrLenPtr := &acceptAddrLen
		backlog := sys.MaxListenerBacklog()
		fd.acceptFuture = &AcceptFuture{
			op:         NewOperation(backlog),
			ln:         fd,
			addr:       acceptAddr,
			addrLen:    acceptAddrLenPtr,
			submitOnce: sync.Once{},
			err:        nil,
		}
	}
}

func (fd *ListenerFd) Accept() (nfd *ConnFd, err error) {
	if fd.acceptFuture == nil {
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
	} else {
		nfd, _, err = fd.acceptFuture.Await()
	}
	return
}

func (fd *ListenerFd) Close() error {
	if fd.acceptFuture != nil {
		_ = fd.acceptFuture.Cancel()
	}
	return fd.NetFd.Close()
}

func (fd *ListenerFd) newAcceptedConnFd(accepted int) (cfd *ConnFd) {
	cfd = &ConnFd{
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

type AcceptFuture struct {
	ln         *ListenerFd
	op         *Operation
	addr       *syscall.RawSockaddrAny
	addrLen    *int
	submitOnce sync.Once
	err        error
}

func (f *AcceptFuture) submit() {
	f.submitOnce.Do(func() {
		alloc := f.ln.Registered()
		op := f.op
		op.Hijack()
		op.WithDirectAlloc(alloc).PrepareAcceptMultishot(f.ln, f.addr, f.addrLen)
		if ok := f.ln.vortex.Submit(op); !ok {
			op.Close()
			f.err = ErrCanceled
			f.op = nil
		}
	})
}

func (f *AcceptFuture) Await() (fd *ConnFd, cqeFlags uint32, err error) {
	f.submit()
	if f.err != nil {
		err = f.err
		return
	}
	var (
		op       = f.op
		ln       = f.ln
		deadline = ln.readDeadline
		vortex   = f.ln.vortex
		accepted = -1
	)
	accepted, cqeFlags, err = vortex.awaitOperationWithDeadline(op, deadline)
	if err != nil {
		f.err = err
		return
	}
	if cqeFlags&liburing.IORING_CQE_F_MORE == 0 {
		f.err = ErrCanceled
		err = f.err
		return
	}
	fd = ln.newAcceptedConnFd(accepted)
	return
}

func (f *AcceptFuture) Cancel() (err error) {
	if f.op != nil {
		op := f.op
		f.op = nil
		err = f.ln.vortex.CancelOperation(op)
		op.Close()
	}
	return
}
