//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"net"
)

func (c *Conn) Send(b []byte) (n int, err error) {
	if c.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}
	if c.sendZCEnabled {
		n, err = c.sendZC(b)
		return
	}
	op := c.vortex.acquireOperation()
	op.WithDeadline(c.writeDeadline).PrepareSend(c, b)
	n, _, err = c.vortex.submitAndWait(op)
	c.vortex.releaseOperation(op)
	if err != nil {
		return
	}
	return
}

func (c *Conn) sendZC(b []byte) (n int, err error) {
	op := c.vortex.acquireOperation()
	op.Hijack()
	op.WithDeadline(c.writeDeadline).PrepareSendZC(c, b)
	var (
		cqeFlags uint32
	)
	n, cqeFlags, err = c.vortex.submitAndWait(op)
	if err != nil {
		op.Complete()
		c.vortex.releaseOperation(op)
		return
	}

	if cqeFlags&liburing.IORING_CQE_F_MORE != 0 {
		_, cqeFlags, err = c.vortex.awaitOperation(op)
		if err != nil {
			op.Complete()
			c.vortex.releaseOperation(op)
			return
		}
		if cqeFlags&liburing.IORING_CQE_F_NOTIF == 0 {
			err = errors.New("send_zc received CQE_F_MORE but no CQE_F_NOTIF")
		}
	}
	op.Complete()
	c.vortex.releaseOperation(op)
	return
}

func (c *Conn) SendTo(b []byte, addr net.Addr) (n int, err error) {
	if c.sendMSGZCEnabled {
		n, err = c.sendToZC(b, addr)
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
	msg := c.vortex.acquireMsg(b, nil, rsa, int(rsaLen), 0)
	defer c.vortex.releaseMsg(msg)

	op := c.vortex.acquireOperation()
	op.WithDeadline(c.writeDeadline).PrepareSendMsg(c, msg)
	n, _, err = c.vortex.submitAndWait(op)
	c.vortex.releaseOperation(op)
	return
}

func (c *Conn) sendToZC(b []byte, addr net.Addr) (n int, err error) {
	n, _, err = c.sendMsgZC(b, nil, addr)
	return
}

func (c *Conn) SendMsg(b []byte, oob []byte, addr net.Addr) (n int, oobn int, err error) {
	if c.sendMSGZCEnabled {
		n, oobn, err = c.sendMsgZC(b, oob, addr)
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
	msg := c.vortex.acquireMsg(b, oob, rsa, int(rsaLen), 0)
	defer c.vortex.releaseMsg(msg)

	op := c.vortex.acquireOperation()
	op.WithDeadline(c.writeDeadline).PrepareSendMsg(c, msg)
	n, _, err = c.vortex.submitAndWait(op)
	if err == nil {
		oobn = int(msg.Controllen)
	}
	c.vortex.releaseOperation(op)
	return
}

func (c *Conn) sendMsgZC(b []byte, oob []byte, addr net.Addr) (n int, oobn int, err error) {
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
	msg := c.vortex.acquireMsg(b, oob, rsa, int(rsaLen), 0)
	defer c.vortex.releaseMsg(msg)

	op := c.vortex.acquireOperation()
	op.Hijack()
	op.WithDeadline(c.writeDeadline).PrepareSendMsgZC(c, msg)
	var (
		cqeFlags uint32
	)
	n, cqeFlags, err = c.vortex.submitAndWait(op)
	if err != nil {
		op.Complete()
		c.vortex.releaseOperation(op)
		return
	}

	oobn = int(msg.Controllen)

	if cqeFlags&liburing.IORING_CQE_F_MORE != 0 {
		_, cqeFlags, err = c.vortex.awaitOperation(op)
		if err != nil {
			op.Complete()
			c.vortex.releaseOperation(op)
			return
		}
		if cqeFlags&liburing.IORING_CQE_F_NOTIF == 0 {
			err = errors.New("sendmsg_zc received CQE_F_MORE but no CQE_F_NOTIF")
		}
	}
	op.Complete()
	c.vortex.releaseOperation(op)
	return
}
