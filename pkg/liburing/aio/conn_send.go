//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"net"
)

func (c *Conn) Send(b []byte) (n int, err error) {
	if c.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}

	op := c.eventLoop.resource.AcquireOperation()
	op.WithDeadline(c.eventLoop.resource, c.writeDeadline)
	if c.sendZCEnabled {
		op.PrepareSendZC(c, b)
		op.Hijack()
	} else {
		op.PrepareSend(c, b)
	}

	n, _, err = c.eventLoop.SubmitAndWait(op)
	if c.sendZCEnabled {
		op.Complete()
	}
	c.eventLoop.resource.ReleaseOperation(op)
	if err != nil {
		return
	}
	return
}

func (c *Conn) SendTo(b []byte, addr net.Addr) (n int, err error) {
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

	msg := c.eventLoop.resource.AcquireMsg(b, nil, rsa, int(rsaLen), 0)

	op := c.eventLoop.resource.AcquireOperation()
	op.WithDeadline(c.eventLoop.resource, c.writeDeadline)
	if c.sendMSGZCEnabled {
		op.PrepareSendMsgZC(c, msg)
		op.Hijack()
	} else {
		op.PrepareSendMsg(c, msg)
	}
	n, _, err = c.eventLoop.SubmitAndWait(op)
	if c.sendMSGZCEnabled {
		op.Complete()
	}

	c.eventLoop.resource.ReleaseOperation(op)
	c.eventLoop.resource.ReleaseMsg(msg)
	return
}

func (c *Conn) SendMsg(b []byte, oob []byte, addr net.Addr) (n int, oobn int, err error) {
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
	msg := c.eventLoop.resource.AcquireMsg(b, oob, rsa, int(rsaLen), 0)

	op := c.eventLoop.resource.AcquireOperation()
	op.WithDeadline(c.eventLoop.resource, c.writeDeadline)
	if c.sendMSGZCEnabled {
		op.PrepareSendMsgZC(c, msg)
		op.Hijack()
	} else {
		op.PrepareSendMsg(c, msg)
	}
	n, _, err = c.eventLoop.SubmitAndWait(op)
	if c.sendMSGZCEnabled {
		op.Complete()
	}
	if err == nil {
		oobn = int(msg.Controllen)
	}

	c.eventLoop.resource.ReleaseOperation(op)
	c.eventLoop.resource.ReleaseMsg(msg)
	return
}
