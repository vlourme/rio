//go:build linux

package aio

type Conn struct {
	NetFd
	multishotReceiver *MultishotReceiver
}

func (c *Conn) tryReleaseMultishotReceiver() {
	if c.multishotReceiver != nil {
		_ = c.multishotReceiver.Close()
	}
}

func (c *Conn) Close() error {
	c.tryReleaseMultishotReceiver()
	return c.NetFd.Close()
}

func (c *Conn) CloseRead() error {
	c.tryReleaseMultishotReceiver()
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
