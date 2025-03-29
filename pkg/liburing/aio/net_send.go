//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"net"
)

func (fd *ConnFd) Send(b []byte) (n int, err error) {
	if fd.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}
	if fd.sendMSGZCEnabled {
		n, err = fd.sendZC(b)
		return
	}
	op := fd.vortex.acquireOperation()
	op.WithDeadline(fd.writeDeadline).PrepareSend(fd, b)
	n, _, err = fd.vortex.submitAndWait(op)
	fd.vortex.releaseOperation(op)
	if err != nil {
		return
	}
	return
}

func (fd *ConnFd) sendZC(b []byte) (n int, err error) {
	op := fd.vortex.acquireOperation()
	op.Hijack()
	op.WithDeadline(fd.writeDeadline).PrepareSendZC(fd, b)
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

func (fd *ConnFd) SendTo(b []byte, addr net.Addr) (n int, err error) {
	if fd.sendMSGZCEnabled {
		n, err = fd.sendToZC(b, addr)
		return
	}
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
	op.WithDeadline(fd.writeDeadline).PrepareSendMsg(fd, msg)
	n, _, err = fd.vortex.submitAndWait(op)
	fd.vortex.releaseOperation(op)
	return
}

func (fd *ConnFd) sendToZC(b []byte, addr net.Addr) (n int, err error) {
	n, _, err = fd.sendMsgZC(b, nil, addr)
	return
}

func (fd *ConnFd) SendMsg(b []byte, oob []byte, addr net.Addr) (n int, oobn int, err error) {
	if fd.sendMSGZCEnabled {
		n, oobn, err = fd.sendMsgZC(b, oob, addr)
		return
	}
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
	op.WithDeadline(fd.writeDeadline).PrepareSendMsg(fd, msg)
	n, _, err = fd.vortex.submitAndWait(op)
	if err == nil {
		oobn = int(msg.Controllen)
	}
	fd.vortex.releaseOperation(op)
	return
}

func (fd *ConnFd) sendMsgZC(b []byte, oob []byte, addr net.Addr) (n int, oobn int, err error) {
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
	op.WithDeadline(fd.writeDeadline).PrepareSendMsgZC(fd, msg)
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
