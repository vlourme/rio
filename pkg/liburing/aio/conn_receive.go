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
	op := c.vortex.acquireOperation()
	op.WithDeadline(c.readDeadline).PrepareReceive(c, b)
	n, _, err = c.vortex.submitAndWait(op)
	c.vortex.releaseOperation(op)
	if n == 0 && err == nil && c.ZeroReadIsEOF() {
		err = io.EOF
	}
	return
}

func (c *Conn) ReceiveFrom(b []byte) (n int, addr net.Addr, err error) {
	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	msg := c.vortex.acquireMsg(b, nil, rsa, rsaLen, 0)
	defer c.vortex.releaseMsg(msg)

	op := c.vortex.acquireOperation()
	op.WithDeadline(c.readDeadline).PrepareReceiveMsg(c, msg)
	n, _, err = c.vortex.submitAndWait(op)
	c.vortex.releaseOperation(op)
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

	msg := c.vortex.acquireMsg(b, oob, rsa, rsaLen, int32(flags))
	defer c.vortex.releaseMsg(msg)

	op := c.vortex.acquireOperation()
	op.WithDeadline(c.readDeadline).PrepareReceiveMsg(c, msg)
	n, _, err = c.vortex.submitAndWait(op)
	if err == nil {
		oobn = int(msg.Controllen)
		flag = int(msg.Flags)
	}
	c.vortex.releaseOperation(op)
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
