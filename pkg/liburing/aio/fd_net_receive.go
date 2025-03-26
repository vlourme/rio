//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"io"
	"net"
	"syscall"
	"time"
)

func (fd *NetFd) Receive(b []byte, deadline time.Time) (n int, err error) {
	if fd.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}

	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReceive(fd, b)
	n, _, err = fd.vortex.submitAndWait(op)
	fd.vortex.releaseOperation(op)
	if n == 0 && err == nil && fd.ZeroReadIsEOF() {
		err = io.EOF
	}
	return
}

func (fd *NetFd) ReceiveFrom(b []byte, deadline time.Time) (n int, addr net.Addr, err error) {
	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	msg := fd.vortex.acquireMsg(b, nil, rsa, rsaLen, 0)
	defer fd.vortex.releaseMsg(msg)

	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReceiveMsg(fd, msg)
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

func (fd *NetFd) ReceiveMsg(b []byte, oob []byte, flags int, deadline time.Time) (n int, oobn int, flag int, addr net.Addr, err error) {
	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	msg := fd.vortex.acquireMsg(b, oob, rsa, rsaLen, 0)
	defer fd.vortex.releaseMsg(msg)

	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReceiveMsg(fd, msg)
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
