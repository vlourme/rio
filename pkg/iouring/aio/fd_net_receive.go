//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring/aio/sys"
	"io"
	"net"
	"os"
	"syscall"
	"time"
)

func (fd *NetFd) Receive(b []byte, deadline time.Time) (n int, err error) {
	if fd.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}
	if fd.canInAdvance() {
		n, err = syscall.Read(fd.regular, b)
		if err == nil {
			if n == 0 && fd.ZeroReadIsEOF() {
				err = io.EOF
			}
			return
		}
		n = 0
		if !errors.Is(err, syscall.EAGAIN) {
			if errors.Is(err, syscall.ECANCELED) {
				err = net.ErrClosed
			} else {
				err = os.NewSyscallError("read", err)
			}
			return
		}
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
	if fd.canInAdvance() {
		var sa syscall.Sockaddr
		n, sa, err = syscall.Recvfrom(fd.regular, b, 0)
		if err == nil {
			addr = sys.SockaddrToAddr(fd.Net(), sa)
			return
		}
		n = 0
		if !errors.Is(err, syscall.EAGAIN) {
			if errors.Is(err, syscall.ECANCELED) {
				err = net.ErrClosed
			} else {
				err = os.NewSyscallError("recvfrom", err)
			}
			return
		}
	}

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
	if fd.canInAdvance() {
		var sa syscall.Sockaddr
		n, oobn, flag, sa, err = syscall.Recvmsg(fd.regular, b, oob, flags)
		if err == nil {
			addr = sys.SockaddrToAddr(fd.Net(), sa)
			return
		}
		n = 0
		if !errors.Is(err, syscall.EAGAIN) {
			if errors.Is(err, syscall.ECANCELED) {
				err = net.ErrClosed
			} else {
				err = os.NewSyscallError("recvmsg", err)
			}
			return
		}
	}
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
