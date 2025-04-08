//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"io"
	"net"
	"syscall"
)

func (c *Conn) Receive(b []byte) (n int, err error) {
	if c.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}
	n, err = c.recvFn(b)
	return
}

func (c *Conn) receive(b []byte) (n int, err error) {
	op := c.eventLoop.resource.AcquireOperation()
	op.WithDeadline(c.eventLoop.resource, c.readDeadline).PrepareReceive(c, b)
	n, _, err = c.eventLoop.SubmitAndWait(op)
	c.eventLoop.resource.ReleaseOperation(op)
	if n == 0 && err == nil && c.ZeroReadIsEOF() {
		err = io.EOF
	}
	return
}

func (c *Conn) ReceiveFrom(b []byte) (n int, addr net.Addr, err error) {
	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	msg := c.eventLoop.resource.AcquireMsg(b, nil, rsa, rsaLen, 0)

	op := c.eventLoop.resource.AcquireOperation()
	op.WithDeadline(c.eventLoop.resource, c.readDeadline).PrepareReceiveMsg(c, msg)
	n, _, err = c.eventLoop.SubmitAndWait(op)
	c.eventLoop.resource.ReleaseOperation(op)

	c.eventLoop.resource.ReleaseMsg(msg)
	if err != nil {
		return
	}
	sa, saErr := sys.RawSockaddrAnyToSockaddr(rsa)
	if saErr != nil {
		err = saErr
		return
	}
	addr = sys.SockaddrToAddr(c.Net(), sa)
	return
}

func (c *Conn) ReceiveMsg(b []byte, oob []byte, flags int) (n int, oobn int, flag int, addr net.Addr, err error) {
	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	msg := c.eventLoop.resource.AcquireMsg(b, oob, rsa, rsaLen, int32(flags))

	op := c.eventLoop.resource.AcquireOperation()
	op.WithDeadline(c.eventLoop.resource, c.readDeadline).PrepareReceiveMsg(c, msg)
	n, _, err = c.eventLoop.SubmitAndWait(op)
	if err == nil {
		oobn = int(msg.Controllen)
		flag = int(msg.Flags)
	}
	c.eventLoop.resource.ReleaseOperation(op)

	c.eventLoop.resource.ReleaseMsg(msg)

	if err != nil {
		return
	}
	sa, saErr := sys.RawSockaddrAnyToSockaddr(rsa)
	if saErr != nil {
		err = saErr
		return
	}
	addr = sys.SockaddrToAddr(c.Net(), sa)
	return
}
