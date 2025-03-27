//go:build linux

package aio

import (
	"syscall"
	"time"
)

func newAcceptedNetFd(ln *NetFd, accepted int, directAllocated bool) (fd *NetFd, err error) {
	vortex := ln.vortex
	fd = &NetFd{
		Fd: Fd{
			regular:       -1,
			direct:        -1,
			isStream:      ln.isStream,
			zeroReadIsEOF: ln.zeroReadIsEOF,
			vortex:        vortex,
		},
		family: ln.family,
		sotype: ln.sotype,
		net:    ln.net,
		laddr:  nil,
		raddr:  nil,
	}
	if directAllocated {
		fd.direct = accepted
	} else {
		fd.regular = accepted
	}
	return
}

func (fd *NetFd) Accept(addr *syscall.RawSockaddrAny, addrLen *int, deadline time.Time) (conn *NetFd, err error) {
	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareAccept(fd, addr, addrLen)
	accepted, _, acceptErr := fd.vortex.submitAndWait(op)
	fd.vortex.releaseOperation(op)
	if acceptErr != nil {
		err = acceptErr
		return
	}
	conn, err = newAcceptedNetFd(fd, accepted, false)
	return
}

func (fd *NetFd) AcceptDirectAlloc(addr *syscall.RawSockaddrAny, addrLen *int, deadline time.Time) (conn *NetFd, err error) {
	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).WithDirect(true).PrepareAccept(fd, addr, addrLen)
	accepted, _, acceptErr := fd.vortex.submitAndWait(op)
	fd.vortex.releaseOperation(op)
	if acceptErr != nil {
		err = acceptErr
		return
	}
	conn, err = newAcceptedNetFd(fd, accepted, true)
	return
}

func (fd *NetFd) AcceptMultishotAsync(buffer int) *AcceptFuture {
	addr := &syscall.RawSockaddrAny{}
	addrLen := syscall.SizeofSockaddrAny
	addrLenPtr := &addrLen
	op := NewOperation(buffer)
	op.Hijack()
	op.PrepareAcceptMultishot(fd, addr, addrLenPtr)
	if ok := fd.vortex.Submit(op); ok {
		return &AcceptFuture{
			vortex:      fd.vortex,
			op:          op,
			ln:          fd,
			addr:        addr,
			addrLen:     addrLenPtr,
			directAlloc: false,
		}
	} else {
		return &AcceptFuture{err: ErrCanceled}
	}
}

func (fd *NetFd) AcceptMultishotDirectAsync(buffer int) *AcceptFuture {
	addr := &syscall.RawSockaddrAny{}
	addrLen := syscall.SizeofSockaddrAny
	addrLenPtr := &addrLen
	op := NewOperation(buffer)
	op.Hijack()
	op.WithDirect(true).PrepareAcceptMultishot(fd, addr, addrLenPtr)
	if ok := fd.vortex.Submit(op); ok {
		return &AcceptFuture{
			vortex:      fd.vortex,
			op:          op,
			ln:          fd,
			addr:        addr,
			addrLen:     addrLenPtr,
			directAlloc: true,
		}
	} else {
		return &AcceptFuture{err: ErrCanceled}
	}
}

type AcceptFuture struct {
	vortex      *Vortex
	op          *Operation
	ln          *NetFd
	addr        *syscall.RawSockaddrAny
	addrLen     *int
	directAlloc bool
	err         error
}

func (f *AcceptFuture) Operation() *Operation {
	return f.op
}

func (f *AcceptFuture) Await() (fd *NetFd, cqeFlags uint32, err error) {
	if f.err != nil {
		err = f.err
		return
	}
	var (
		op          = f.op
		ln          = f.ln
		accepted    = -1
		directAlloc = f.directAlloc
	)
	accepted, cqeFlags, err = f.vortex.awaitOperation(op) // todo handle timeout...
	f.vortex.releaseOperation(op)
	if err != nil {
		return
	}
	fd, err = newAcceptedNetFd(ln, accepted, directAlloc)
	return
}

func (f *AcceptFuture) Cancel() (err error) {
	err = f.vortex.CancelOperation(f.op)
	return
}
