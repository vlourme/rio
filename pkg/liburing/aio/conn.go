//go:build linux

package aio

import (
	"syscall"
)

type Conn struct {
	NetFd
	recvFn  func([]byte) (int, error)
	handler *RecvMultishotHandler
}

func (c *Conn) init() {
	switch c.sotype {
	case syscall.SOCK_STREAM: // multi recv
		if c.multishot {
			handler, handlerErr := newRecvMultishotHandler(c)
			if handlerErr == nil {
				c.handler = handler
				c.recvFn = c.handler.Receive
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

func (c *Conn) Close() error {
	if c.handler != nil {
		_ = c.handler.Close()
	}
	return c.NetFd.Close()
}

func (c *Conn) CloseRead() error {
	if c.handler != nil {
		_ = c.handler.Close()
	}
	if c.Registered() {
		op := c.eventLoop.resource.AcquireOperation()
		op.PrepareCloseRead(c)
		_, _, err := c.eventLoop.SubmitAndWait(op)
		c.eventLoop.resource.ReleaseOperation(op)
		return err
	}
	return syscall.Shutdown(c.regular, syscall.SHUT_RD)
}

func (c *Conn) CloseWrite() error {
	if c.Registered() {
		op := c.eventLoop.resource.AcquireOperation()
		op.PrepareCloseWrite(c)
		_, _, err := c.eventLoop.SubmitAndWait(op)
		c.eventLoop.resource.ReleaseOperation(op)
		return err
	}
	return syscall.Shutdown(c.regular, syscall.SHUT_WR)
}
