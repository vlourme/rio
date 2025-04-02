//go:build linux

package aio

import (
	"net"
	"syscall"
)

type Conn struct {
	NetFd
	sendZCEnabled    bool
	sendMSGZCEnabled bool
	recvFn           func([]byte) (int, error)
	recvFuture       *receiveFuture
}

func (c *Conn) init() {
	switch c.sotype {
	case syscall.SOCK_STREAM: // multi recv
		if c.vortex.multishotReceiveEnabled() {
			futureErr := newReceiveFuture(c)
			if futureErr == nil {
				c.recvFn = c.recvFuture.receive
			} else {
				c.recvFn = c.receive
			}
		} else {
			c.recvFn = c.receive
		}
		break
	case syscall.SOCK_DGRAM: // todo multi recv msg
		c.recvFn = c.receive
		break
	default:
		c.recvFn = c.receive
		break
	}
	return
}

func (c *Conn) Bind(addr net.Addr) error {
	return c.bind(addr)
}

func (c *Conn) SendZCEnabled() bool {
	return c.sendZCEnabled
}

func (c *Conn) SendMSGZCEnabled() bool {
	return c.sendMSGZCEnabled
}

func (c *Conn) Close() error {
	if c.recvFuture != nil {
		_ = c.recvFuture.Cancel()
	}
	return c.Fd.Close()
}

func (c *Conn) CloseRead() error {
	if c.recvFuture != nil {
		_ = c.recvFuture.Cancel()
	}
	if c.Registered() {
		op := c.vortex.acquireOperation()
		op.PrepareCloseRead(c)
		_, _, err := c.vortex.submitAndWait(op)
		c.vortex.releaseOperation(op)
		return err
	}
	return syscall.Shutdown(c.regular, syscall.SHUT_RD)
}

func (c *Conn) CloseWrite() error {
	if c.Registered() {
		op := c.vortex.acquireOperation()
		op.PrepareCloseWrite(c)
		_, _, err := c.vortex.submitAndWait(op)
		c.vortex.releaseOperation(op)
		return err
	}
	return syscall.Shutdown(c.regular, syscall.SHUT_WR)
}
