//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"syscall"
	"time"
)

func (fd *NetFd) Receive(b []byte, deadline time.Time) (n int, err error) {
	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReceive(fd, b)
	n, _, err = fd.vortex.submitAndWait(fd.ctx, op)
	fd.vortex.releaseOperation(op)
	return
}

func (fd *NetFd) Send(b []byte, deadline time.Time) (n int, err error) {
	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSend(fd, b)
	n, _, err = fd.vortex.submitAndWait(fd.ctx, op)
	fd.vortex.releaseOperation(op)
	return
}

func (fd *NetFd) SendZC(b []byte, deadline time.Time) (n int, err error) {
	op := fd.vortex.acquireOperation()
	op.Hijack()
	op.WithDeadline(deadline).PrepareSendZC(fd, b)
	var (
		cqeFlags uint32
	)
	n, cqeFlags, err = fd.vortex.submitAndWait(fd.ctx, op)
	if err != nil {
		op.Complete()
		fd.vortex.releaseOperation(op)
		return
	}

	if cqeFlags&iouring.CQEFMore != 0 {
		_, cqeFlags, err = fd.vortex.awaitOperation(fd.ctx, op)
		if err != nil {
			op.Complete()
			fd.vortex.releaseOperation(op)
			return
		}
		if cqeFlags&iouring.CQEFNotify == 0 {
			err = errors.New("send_zc received CQE_F_MORE but no CQE_F_NOTIF")
		}
	}
	op.Complete()
	fd.vortex.releaseOperation(op)
	return
}

func (fd *NetFd) ReceiveFrom(b []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (n int, err error) {
	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReceiveMsg(fd, b, nil, addr, addrLen, 0)
	n, _, err = fd.vortex.submitAndWait(fd.ctx, op)
	fd.vortex.releaseOperation(op)
	return
}

func (fd *NetFd) SendTo(b []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (n int, err error) {
	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendMsg(fd, b, nil, addr, addrLen, 0)
	n, _, err = fd.vortex.submitAndWait(fd.ctx, op)
	fd.vortex.releaseOperation(op)
	return
}

func (fd *NetFd) SendToZC(b []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (n int, err error) {
	n, _, err = fd.SendMsgZC(b, nil, addr, addrLen, deadline)
	return
}

func (fd *NetFd) ReceiveMsg(b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int, deadline time.Time) (n int, oobn int, flag int, err error) {
	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReceiveMsg(fd, b, oob, addr, addrLen, int32(flags))
	n, _, err = fd.vortex.submitAndWait(fd.ctx, op)
	if err == nil {
		oobn = int(op.msg.Controllen)
		flag = int(op.msg.Flags)
	}
	fd.vortex.releaseOperation(op)
	return
}

func (fd *NetFd) SendMsg(b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (n int, oobn int, err error) {
	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendMsg(fd, b, oob, addr, addrLen, 0)
	n, _, err = fd.vortex.submitAndWait(fd.ctx, op)
	if err == nil {
		oobn = int(op.msg.Controllen)
	}
	fd.vortex.releaseOperation(op)
	return
}

func (fd *NetFd) SendMsgZC(b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (n int, oobn int, err error) {
	op := fd.vortex.acquireOperation()
	op.Hijack()
	op.WithDeadline(deadline).PrepareSendMsgZC(fd, b, oob, addr, addrLen, 0)
	var (
		cqeFlags uint32
	)
	n, cqeFlags, err = fd.vortex.submitAndWait(fd.ctx, op)
	if err != nil {
		op.Complete()
		fd.vortex.releaseOperation(op)
		return
	}

	oobn = int(op.msg.Controllen)

	if cqeFlags&iouring.CQEFMore != 0 {
		_, cqeFlags, err = fd.vortex.awaitOperation(fd.ctx, op)
		if err != nil {
			op.Complete()
			fd.vortex.releaseOperation(op)
			return
		}
		if cqeFlags&iouring.CQEFNotify == 0 {
			err = errors.New("sendmsg_zc received CQE_F_MORE but no CQE_F_NOTIF")
		}
	}
	op.Complete()
	fd.vortex.releaseOperation(op)
	return
}
