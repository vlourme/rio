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

	op := AcquireDeadlineOperation(c.writeDeadline)
	if c.sendZCEnabled {
		op.PrepareSendZC(c, b)
	} else {
		op.PrepareSend(c, b)
	}
	n, _, err = c.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
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

	msg := acquireMsg(b, nil, rsa, int(rsaLen), 0)
	op := AcquireDeadlineOperation(c.writeDeadline)
	if c.sendMSGZCEnabled {
		op.PrepareSendMsgZC(c, msg)
	} else {
		op.PrepareSendMsg(c, msg)
	}
	n, _, err = c.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
	releaseMsg(msg)
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
	msg := acquireMsg(b, oob, rsa, int(rsaLen), 0)
	op := AcquireDeadlineOperation(c.writeDeadline)
	if c.sendMSGZCEnabled {
		op.PrepareSendMsgZC(c, msg)
	} else {
		op.PrepareSendMsg(c, msg)
	}
	n, _, err = c.eventLoop.SubmitAndWait(op)
	if err == nil {
		oobn = int(msg.Controllen)
	}
	ReleaseOperation(op)
	releaseMsg(msg)
	return
}
