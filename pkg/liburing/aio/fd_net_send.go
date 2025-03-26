//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"net"
	"time"
)

func (fd *NetFd) Send(b []byte, deadline time.Time) (n int, err error) {
	if fd.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}
	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSend(fd, b)
	n, _, err = fd.vortex.submitAndWait(op)
	fd.vortex.releaseOperation(op)
	if err != nil {
		return
	}
	return
}

func (fd *NetFd) SendZC(b []byte, deadline time.Time) (n int, err error) {
	op := fd.vortex.acquireOperation()
	op.Hijack()
	op.WithDeadline(deadline).PrepareSendZC(fd, b)
	var (
		cqeFlags uint32
	)
	n, cqeFlags, err = fd.vortex.submitAndWait(op)
	if err != nil {
		op.Complete()
		fd.vortex.releaseOperation(op)
		return
	}

	if cqeFlags&liburing.IORING_CQE_F_MORE != 0 {
		_, cqeFlags, err = fd.vortex.awaitOperation(op)
		if err != nil {
			op.Complete()
			fd.vortex.releaseOperation(op)
			return
		}
		if cqeFlags&liburing.IORING_CQE_F_NOTIF == 0 {
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
	rsa, rsaLen, rsaErr := sys.SockaddrToRawSockaddrAny(sa)
	if rsaErr != nil {
		err = rsaErr
		return
	}
	msg := fd.vortex.acquireMsg(b, nil, rsa, int(rsaLen), 0)
	defer fd.vortex.releaseMsg(msg)

	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendMsg(fd, msg)
	n, _, err = fd.vortex.submitAndWait(op)
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
	rsa, rsaLen, rsaErr := sys.SockaddrToRawSockaddrAny(sa)
	if rsaErr != nil {
		err = rsaErr
		return
	}
	msg := fd.vortex.acquireMsg(b, oob, rsa, int(rsaLen), 0)
	defer fd.vortex.releaseMsg(msg)

	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendMsg(fd, msg)
	n, _, err = fd.vortex.submitAndWait(op)
	if err == nil {
		oobn = int(msg.Controllen)
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
	msg := fd.vortex.acquireMsg(b, oob, rsa, int(rsaLen), 0)
	defer fd.vortex.releaseMsg(msg)

	op := fd.vortex.acquireOperation()
	op.Hijack()
	op.WithDeadline(deadline).PrepareSendMsgZC(fd, msg)
	var (
		cqeFlags uint32
	)
	n, cqeFlags, err = fd.vortex.submitAndWait(op)
	if err != nil {
		op.Complete()
		fd.vortex.releaseOperation(op)
		return
	}

	oobn = int(msg.Controllen)

	if cqeFlags&liburing.IORING_CQE_F_MORE != 0 {
		_, cqeFlags, err = fd.vortex.awaitOperation(op)
		if err != nil {
			op.Complete()
			fd.vortex.releaseOperation(op)
			return
		}
		if cqeFlags&liburing.IORING_CQE_F_NOTIF == 0 {
			err = errors.New("sendmsg_zc received CQE_F_MORE but no CQE_F_NOTIF")
		}
	}
	op.Complete()
	fd.vortex.releaseOperation(op)
	return
}
