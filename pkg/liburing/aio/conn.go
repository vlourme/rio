//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing/aio/bytebuffer"
	"sync"
)

type Conn struct {
	NetFd
	prepareRecvMultishotOnce   sync.Once
	recvMultishotBufferAndRing *BufferAndRing
	recvMultishotOperation     *Operation
	recvMultishotFuture        Future
	recvMultishotBuffer        *bytebuffer.Buffer
}

func (c *Conn) checkMultishot() {
	if c.multishot {
		br, brErr := c.eventLoop.AcquireBufferAndRing()
		if brErr != nil {
			c.multishot = false
			return
		}
		c.recvMultishotBufferAndRing = br
		c.recvMultishotBuffer = bytebuffer.Acquire()
	}
}

func (c *Conn) releaseRecvMultishot() {
	if c.recvMultishotOperation != nil {
		_ = c.eventLoop.Cancel(c.recvMultishotOperation)
		_, _, _, _ = c.recvMultishotFuture.Await()
		op := c.recvMultishotOperation
		c.recvMultishotOperation = nil
		c.recvMultishotFuture = nil
		ReleaseOperation(op)
	}
}

func (c *Conn) closeRecvMultishot() {
	c.releaseRecvMultishot()
	if c.recvMultishotBuffer != nil {
		buffer := c.recvMultishotBuffer
		c.recvMultishotBuffer = nil
		bytebuffer.Release(buffer)
	}
}

func (c *Conn) Close() error {
	c.closeRecvMultishot()

	return c.NetFd.Close()
}

func (c *Conn) CloseRead() error {
	c.closeRecvMultishot()

	op := AcquireOperation()
	op.PrepareCloseRead(c)
	_, _, err := c.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
	return err
}

func (c *Conn) CloseWrite() error {
	op := AcquireOperation()
	op.PrepareCloseWrite(c)
	_, _, err := c.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
	return err
}
