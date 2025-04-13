//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/bytebuffer"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"io"
	"net"
	"syscall"
)

func (c *Conn) Receive(b []byte) (n int, err error) {
	if c.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}
	if !c.multishot {
		c.prepareRecvMultishot()
		n, err = c.recvMultishot(b)
		return
	}
	n, err = c.receiveOneshot(b)
	return
}

func (c *Conn) receiveOneshot(b []byte) (n int, err error) {
	op := AcquireDeadlineOperation(c.readDeadline)
	op.PrepareReceive(c, b)
	n, _, err = c.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
	if n == 0 && err == nil && c.ZeroReadIsEOF() {
		err = io.EOF
	}
	return
}

func (c *Conn) recvMultishot(b []byte) (n int, err error) {
	bLen := len(b)
	if c.recvMultishotBuffer.Len() > 0 {
		n, _ = c.recvMultishotBuffer.Read(b)
		if n == bLen {
			if c.recvMultishotOperation != nil {
				goto AWAIT
			}
			return
		}
	}
	if c.recvMultishotOperation == nil {
		if n == 0 && c.zeroReadIsEOF {
			err = io.EOF
		}
		return
	}

AWAIT:
	hungry := n == 0
	events := c.recvMultishotFuture.AwaitMore(hungry, c.readDeadline)
	eventsLen := len(events)
	if eventsLen == 0 {
		return
	}
	for i := 0; i < eventsLen; i++ {
		event := events[i]
		if event.Err != nil {
			if errors.Is(event.Err, syscall.ENOBUFS) { // try to submit again
				c.recvMultishotFuture = c.eventLoop.Submit(c.recvMultishotOperation)
				if n == 0 && i == eventsLen-1 {
					goto AWAIT
				}
				continue
			}
			err = event.Err
			return
		}
		// handle attachment
		if event.Attachment != nil {
			buf := (*bytebuffer.Buffer)(event.Attachment)
			nn, _ := buf.Read(b[n:])
			n += nn
			if buf.Len() > 0 {
				_, _ = buf.WriteTo(c.recvMultishotBuffer)
			}
			bytebuffer.Release(buf)
			event.Attachment = nil
		}
		// handle CQE_F_MORE
		if event.Flags&liburing.IORING_CQE_F_MORE == 0 {
			if event.N == 0 { // eof
				c.releaseRecvMultishot()
				if n == 0 && c.zeroReadIsEOF {
					err = io.EOF
				}
				return
			}
			// resubmit (maybe same as ENOBUFS but advance report via cqe flags)
			c.recvMultishotFuture = c.eventLoop.Submit(c.recvMultishotOperation)
			if n == 0 && i == eventsLen-1 {
				goto AWAIT
			}
		}
	}
	return
}

func (c *Conn) prepareRecvMultishot() {
	c.prepareRecvMultishotOnce.Do(func() {
		c.recvMultishotOperation = AcquireOperation()
		c.recvMultishotOperation.PrepareReceiveMultishot(c, c.recvMultishotBufferAndRing)
		c.recvMultishotFuture = c.eventLoop.Submit(c.recvMultishotOperation)
	})
	return
}

func (c *Conn) ReceiveFrom(b []byte) (n int, addr net.Addr, err error) {
	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	msg := acquireMsg(b, nil, rsa, rsaLen, 0)

	op := AcquireDeadlineOperation(c.readDeadline)
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
	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	msg := acquireMsg(b, oob, rsa, rsaLen, int32(flags))

	op := AcquireDeadlineOperation(c.readDeadline)
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
