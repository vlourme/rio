//go:build linux

package aio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring"
	"syscall"
	"time"
)

func newAcceptedNetFd(ln *NetFd, accepted int, directAllocated bool) (fd *NetFd, err error) {
	ctx, cancel := context.WithCancel(ln.ctx)
	vortex := ln.vortex
	fd = &NetFd{
		Fd: Fd{
			ctx:              ctx,
			cancel:           cancel,
			regular:          -1,
			direct:           -1,
			allocated:        directAllocated,
			isStream:         ln.sotype&syscall.SOCK_STREAM != 0,
			zeroReadIsEOF:    ln.sotype != syscall.SOCK_DGRAM && ln.sotype != syscall.SOCK_RAW,
			async:            ln.async,
			nonBlocking:      ln.nonBlocking,
			disableInAdvance: ln.disableInAdvance,
			vortex:           vortex,
		},
		family: ln.family,
		sotype: ln.sotype,
		net:    ln.net,
		laddr:  nil,
		raddr:  nil,
	}
	if directAllocated {
		fd.direct = accepted

		regular, installErr := vortex.FixedFdInstall(ctx, fd.direct) // todo: dont install here, just when used.
		if installErr != nil {
			_ = fd.Close()
			err = installErr
			return
		}
		fd.regular = regular
	} else {
		fd.regular = accepted
	}
	return
}

func (fd *NetFd) Accept(addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (conn *NetFd, err error) {
	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareAccept(fd, addr, addrLen)
	accepted, _, acceptErr := fd.vortex.submitAndWait(fd.ctx, op)
	fd.vortex.releaseOperation(op)
	if acceptErr != nil {
		err = acceptErr
		return
	}
	conn, err = newAcceptedNetFd(fd, accepted, false)
	return
}

func (fd *NetFd) AcceptDirect(addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time, fileIndex uint32) (conn *NetFd, err error) {
	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).WithFiledIndex(fileIndex).WithDirect(true).PrepareAccept(fd, addr, addrLen)
	accepted, _, acceptErr := fd.vortex.submitAndWait(fd.ctx, op)
	fd.vortex.releaseOperation(op)
	if acceptErr != nil {
		err = acceptErr
		return
	}
	conn, err = newAcceptedNetFd(fd, accepted, fileIndex == iouring.FileIndexAlloc)
	return
}

func (fd *NetFd) AcceptDirectAlloc(addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (conn *NetFd, err error) {
	conn, err = fd.AcceptDirect(addr, addrLen, deadline, iouring.FileIndexAlloc)
	return
}

func (fd *NetFd) AcceptMultishotAsync(addr *syscall.RawSockaddrAny, addrLen int, buffer int) AcceptFuture {
	op := NewOperation(buffer)
	op.Hijack()
	op.PrepareAcceptMultishot(fd, addr, addrLen)
	fd.vortex.Submit(op)
	return AcceptFuture{
		vortex:      fd.vortex,
		op:          op,
		ln:          fd,
		directAlloc: false,
	}
}

func (fd *NetFd) AcceptMultishotDirectAsync(addr *syscall.RawSockaddrAny, addrLen int, buffer int) AcceptFuture {
	op := NewOperation(buffer)
	op.Hijack()
	op.WithDirect(true).PrepareAcceptMultishot(fd, addr, addrLen)
	fd.vortex.Submit(op)
	return AcceptFuture{
		vortex:      fd.vortex,
		op:          op,
		ln:          fd,
		directAlloc: true,
	}
}
