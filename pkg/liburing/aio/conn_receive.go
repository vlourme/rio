//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"io"
	"net"
	"syscall"
)

func (c *Conn) Receive(b []byte) (n int, err error) {
	if c.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}
	if c.multishot {
		if c.multishotReceiver == nil {
			c.multishotReceiver, err = newMultishotReceiver(c)
			if err != nil {
				c.multishot = false
				err = nil
				n, err = c.receiveOneshot(b)
				return
			}
		}
		n, err = c.multishotReceiver.Recv(b, c.readDeadline)
		if err != nil && errors.Is(err, io.EOF) {
			if !c.zeroReadIsEOF {
				err = nil
			}
		}
	} else {
		n, err = c.receiveOneshot(b)
	}
	return
}

func (c *Conn) receiveOneshot(b []byte) (n int, err error) {
	op := AcquireOperationWithDeadline(c.readDeadline)
	op.PrepareReceive(c, b)
	n, _, err = c.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
	if n == 0 && err == nil && c.zeroReadIsEOF {
		err = io.EOF
	}
	return
}

func (c *Conn) ReceiveFrom(b []byte) (n int, addr net.Addr, err error) {
	if c.multishot {
		if c.multishotMsgReceiver == nil {
			c.multishotMsgReceiver, err = newMultishotMsgReceiver(c)
			if err != nil {
				c.multishot = false
				err = nil
				n, addr, err = c.receiveFromOneshot(b)
				return
			}
		}
		var rsa *syscall.RawSockaddrAny
		n, _, _, rsa, err = c.multishotMsgReceiver.ReceiveMsg(b, nil, c.readDeadline)
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
			}
			return
		}
		sa, saErr := sys.RawSockaddrAnyToSockaddr(rsa)
		if saErr != nil {
			err = saErr
			return
		}
		addr = sys.SockaddrToAddr(c.net, sa)
	} else {
		n, addr, err = c.receiveFromOneshot(b)
	}
	return
}

func (c *Conn) receiveFromOneshot(b []byte) (n int, addr net.Addr, err error) {
	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	msg := acquireMsg(b, nil, rsa, rsaLen, 0)

	op := AcquireOperationWithDeadline(c.readDeadline)
	op.PrepareReceiveMsg(c, msg)
	n, _, err = c.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)

	releaseMsg(msg)
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
	if c.multishot && flags == 0 {
		if c.multishotMsgReceiver == nil {
			c.multishotMsgReceiver, err = newMultishotMsgReceiver(c)
			if err != nil {
				c.multishot = false
				err = nil
				n, oobn, flag, addr, err = c.receiveMsgOneshot(b, oob, flags)
				return
			}
		}
		var (
			flags32 int32
			rsa     *syscall.RawSockaddrAny
		)
		n, oobn, flags32, rsa, err = c.multishotMsgReceiver.ReceiveMsg(b, oob, c.readDeadline)
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
			}
			return
		}
		sa, saErr := sys.RawSockaddrAnyToSockaddr(rsa)
		if saErr != nil {
			err = saErr
			return
		}
		addr = sys.SockaddrToAddr(c.net, sa)
		flag = int(flags32)
	} else {
		n, oobn, flag, addr, err = c.receiveMsgOneshot(b, oob, flags)
	}
	return
}

func (c *Conn) receiveMsgOneshot(b []byte, oob []byte, flags int) (n int, oobn int, flag int, addr net.Addr, err error) {
	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	msg := acquireMsg(b, oob, rsa, rsaLen, int32(flags))

	op := AcquireOperationWithDeadline(c.readDeadline)
	op.PrepareReceiveMsg(c, msg)
	n, _, err = c.eventLoop.SubmitAndWait(op)
	if err == nil {
		oobn = int(msg.Controllen)
		flag = int(msg.Flags)
	}
	ReleaseOperation(op)

	releaseMsg(msg)

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

func OOBLen() int {
	return syscall.CmsgLen(syscall.SizeofSockaddrAny) + syscall.SizeofCmsghdr
}
