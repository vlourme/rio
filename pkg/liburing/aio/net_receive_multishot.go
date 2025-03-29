//go:build linux

package aio

import (
	"sync"
)

type receiveFuture struct {
	fd         *ConnFd
	op         *Operation
	buffer     *Buffer
	submitOnce sync.Once
	err        error
}

func (f *receiveFuture) submit() {
	f.submitOnce.Do(func() {
		// provide buffer
		op := f.fd.vortex.acquireOperation()
		_ = op.PrepareProvideBuffers(f.buffer.bgid, f.buffer.iovecs)
		_, _, provideErr := f.fd.vortex.submitAndWait(op)
		f.fd.vortex.releaseOperation(op)
		if provideErr != nil {
			f.err = provideErr
			return
		}
		// recv multishot
		recvOp := f.fd.vortex.acquireMultishotOperation()
		f.op = recvOp
		recvOp.Hijack()
		recvOp.PrepareReceiveMultishot(f.fd, f.buffer.bgid)
		if ok := f.fd.vortex.Submit(recvOp); !ok {
			f.err = ErrCanceled
			recvOp.Close()
			f.op = nil
			f.fd.vortex.releaseMultishotOperation(recvOp)
		}
	})
}

func (f *receiveFuture) Await() (n int, bid int, err error) {
	f.submit()
	if f.err != nil {
		err = f.err
		return
	}
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
	// remove buffer
	buffer := f.buffer
	f.buffer = nil
	op := f.fd.vortex.acquireOperation()
	_ = op.PrepareRemoveBuffers(buffer.bgid, len(buffer.iovecs))
	_, _, _ = f.fd.vortex.submitAndWait(op)
	f.fd.vortex.releaseOperation(op)
	// release buffer
	f.fd.vortex.bufferConfig.ReleaseBuffer(buffer)
	return
}
