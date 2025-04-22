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
	if c.Available() {
		op := AcquireOperation()
		op.PrepareCloseRead(c)
		_, _, err := c.eventLoop.SubmitAndWait(op)
		ReleaseOperation(op)
		return err
	}
	return nil
}

func (c *Conn) CloseWrite() error {
	if c.Available() {
		op := AcquireOperation()
		op.PrepareCloseWrite(c)
		_, _, err := c.eventLoop.SubmitAndWait(op)
		ReleaseOperation(op)
		return err
	}
	return nil
}
