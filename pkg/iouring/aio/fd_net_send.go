//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/iouring/aio/sys"
	"net"
	"os"
	"syscall"
	"time"
)

func (fd *NetFd) Send(b []byte, deadline time.Time) (n int, err error) {
	if fd.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}
	nn := 0
	if fd.canInAdvance() {
		n, err = syscall.Write(fd.regular, b)
		if err == nil {
			return
		}
		nn += n
		n = 0
		if !errors.Is(err, syscall.EAGAIN) {
			if errors.Is(err, syscall.ECANCELED) {
				err = net.ErrClosed
			} else {
				err = os.NewSyscallError("write", err)
			}
			return
		}
	}
	if nn > 0 {
		b = b[nn:]
	}

	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSend(fd, b)
	n, _, err = fd.vortex.submitAndWait(fd.ctx, op)
	fd.vortex.releaseOperation(op)
	if err != nil {
		return
	}
	n += nn
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

func (fd *NetFd) SendTo(b []byte, addr net.Addr, deadline time.Time) (n int, err error) {
	sa, saErr := sys.AddrToSockaddr(addr)
	if saErr != nil {
		err = saErr
		return
	}

	if fd.canInAdvance() {
		err = syscall.Sendto(fd.regular, b, 0, sa)
		if err == nil {
			n = len(b)
			return
		}
		n = 0
		if !errors.Is(err, syscall.EAGAIN) {
			if errors.Is(err, syscall.ECANCELED) {
				err = net.ErrClosed
			} else {
				err = os.NewSyscallError("sendto", err)
			}
			return
		}
	}

	rsa, rsaLen, rsaErr := sys.SockaddrToRawSockaddrAny(sa)
	if rsaErr != nil {
		err = rsaErr
		return
	}
	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendMsg(fd, b, nil, rsa, int(rsaLen), 0)
	n, _, err = fd.vortex.submitAndWait(fd.ctx, op)
	fd.vortex.releaseOperation(op)
	return
}

func (fd *NetFd) SendToZC(b []byte, addr net.Addr, deadline time.Time) (n int, err error) {
	n, _, err = fd.SendMsgZC(b, nil, addr, deadline)
	return
}

func (fd *NetFd) SendMsg(b []byte, oob []byte, addr net.Addr, deadline time.Time) (n int, oobn int, err error) {
	sa, saErr := sys.AddrToSockaddr(addr)
	if saErr != nil {
		err = saErr
		return
	}

	if fd.canInAdvance() {
		n, err = syscall.SendmsgN(fd.regular, b, oob, sa, 0)
		if err == nil {
			oobn = len(oob)
			return
		}
		n = 0
		if !errors.Is(err, syscall.EAGAIN) {
			if errors.Is(err, syscall.ECANCELED) {
				err = net.ErrClosed
			} else {
				err = os.NewSyscallError("sendmsgn", err)
			}
			return
		}
	}

	rsa, rsaLen, rsaErr := sys.SockaddrToRawSockaddrAny(sa)
	if rsaErr != nil {
		err = rsaErr
		return
	}

	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendMsg(fd, b, oob, rsa, int(rsaLen), 0)
	n, _, err = fd.vortex.submitAndWait(fd.ctx, op)
	if err == nil {
		oobn = int(op.msg.Controllen)
	}
	fd.vortex.releaseOperation(op)
	return
}

func (fd *NetFd) SendMsgZC(b []byte, oob []byte, addr net.Addr, deadline time.Time) (n int, oobn int, err error) {
	sa, saErr := sys.AddrToSockaddr(addr)
	if saErr != nil {
		err = saErr
		return
	}
	rsa, rsaLen, rsaErr := sys.SockaddrToRawSockaddrAny(sa)
	if rsaErr != nil {
		err = rsaErr
		return
	}

	op := fd.vortex.acquireOperation()
	op.Hijack()
	op.WithDeadline(deadline).PrepareSendMsgZC(fd, b, oob, rsa, int(rsaLen), 0)
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
