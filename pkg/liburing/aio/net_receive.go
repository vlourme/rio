//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"io"
	"net"
	"syscall"
)

func (fd *ConnFd) Receive(b []byte) (n int, err error) {
	if fd.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}
	n, err = fd.recvFn(b)
	return
}

func (fd *ConnFd) receive(b []byte) (n int, err error) {
	op := fd.vortex.acquireOperation()
	op.WithDeadline(fd.readDeadline).PrepareReceive(fd, b)
	n, _, err = fd.vortex.submitAndWait(op)
	fd.vortex.releaseOperation(op)
	if n == 0 && err == nil && fd.ZeroReadIsEOF() {
		err = io.EOF
	}
	return
}

func (fd *ConnFd) ReceiveFrom(b []byte) (n int, addr net.Addr, err error) {
	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	msg := fd.vortex.acquireMsg(b, nil, rsa, rsaLen, 0)
	defer fd.vortex.releaseMsg(msg)

	op := fd.vortex.acquireOperation()
	op.WithDeadline(fd.readDeadline).PrepareReceiveMsg(fd, msg)
	n, _, err = fd.vortex.submitAndWait(op)
	fd.vortex.releaseOperation(op)
	if err != nil {
		return
	}
	sa, saErr := sys.RawSockaddrAnyToSockaddr(rsa)
	if saErr != nil {
		err = saErr
		return
	}
	addr = sys.SockaddrToAddr(fd.Net(), sa)
	return
}

func (fd *ConnFd) ReceiveMsg(b []byte, oob []byte, flags int) (n int, oobn int, flag int, addr net.Addr, err error) {
	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	msg := fd.vortex.acquireMsg(b, oob, rsa, rsaLen, int32(flags))
	defer fd.vortex.releaseMsg(msg)

	op := fd.vortex.acquireOperation()
	op.WithDeadline(fd.readDeadline).PrepareReceiveMsg(fd, msg)
	n, _, err = fd.vortex.submitAndWait(op)
	if err == nil {
		oobn = int(msg.Controllen)
		flag = int(msg.Flags)
	}
	fd.vortex.releaseOperation(op)
	if err != nil {
		return
	}
	sa, saErr := sys.RawSockaddrAnyToSockaddr(rsa)
	if saErr != nil {
		err = saErr
		return
	}
	addr = sys.SockaddrToAddr(fd.Net(), sa)
	return
}
