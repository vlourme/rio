//go:build linux

package aio

func newReceiveFuture(fd *Conn) (future *receiveFuture, err error) {
	f := &receiveFuture{
		fd: fd,
	}
	err = f.submit()
	if err == nil {
		future = f
	}
	return
}

type receiveFuture struct {
	fd *Conn
	op *Operation
	rb *RingBuffer
}

func (f *receiveFuture) submit() (err error) {
	// rb
	buffer, bufferErr := f.fd.vortex.connRingBufferConfig.AcquireRingBuffer(&f.fd.Fd)
	if bufferErr != nil {
		err = bufferErr
		return
	}
	f.rb = buffer
	// recv multishot
	recvOp := f.fd.vortex.acquireMultishotOperation()
	f.op = recvOp
	recvOp.Hijack()
	recvOp.PrepareReceiveMultishot(f.fd, int(f.rb.bgid))
	if ok := f.fd.vortex.Submit(recvOp); !ok {
		recvOp.Close()
		f.op = nil
		f.fd.vortex.releaseMultishotOperation(recvOp)
		err = ErrCanceled
	}
	return
}

func (f *receiveFuture) receive(b []byte) (n int, err error) {
	// read inbound
	// read full, return
	//
	// check err
	// if err not nil
	// case EOF, if n == 0 , return EOF
	// case not eof, if n > 0, return n, if n == 0, return err
	//
	// if err is nil
	// read ch
	//
	// if not read full from ch, write remains into inbound
	//
	// advance ring buffer
	//
	// if read err, set err (discard syscall.ENOBUFS)
	//
	// return n
	return
}

func (f *receiveFuture) await() (n int, bid int, err error) {
	// todo

	return
}

func (f *receiveFuture) Cancel() (err error) {
	// cancel
	if f.op != nil {
		op := f.op
		f.op = nil
		err = f.fd.vortex.CancelOperation(op)
		op.Close()
		f.fd.vortex.releaseMultishotOperation(op)
	}
	// release ring buffer
	if f.rb != nil {
		buffer := f.rb
		f.rb = nil
		_ = f.fd.vortex.connRingBufferConfig.ReleaseRingBuffer(buffer)
	}

	return
}
