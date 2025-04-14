//go:build linux

package aio

type Conn struct {
	NetFd
	recvMultishotAdaptor *RecvMultishotAdaptor
}

func (c *Conn) releaseRecvMultishotAdaptor() {
	c.locker.Lock()
	if c.recvMultishotAdaptor != nil {
		adaptor := c.recvMultishotAdaptor
		c.recvMultishotAdaptor = nil
		_ = adaptor.Close()
		releaseRecvMultishotAdaptor(adaptor)
	}
	c.locker.Unlock()
}

func (c *Conn) Close() error {
	c.releaseRecvMultishotAdaptor()
	return c.NetFd.Close()
}

func (c *Conn) CloseRead() error {
	c.releaseRecvMultishotAdaptor()

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
