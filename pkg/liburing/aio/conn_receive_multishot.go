//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/bytebuffer"
	"io"
	"sync"
	"syscall"
)

type RecvMultishotInbound struct {
	locker  sync.Mutex
	waiting bool
	ch      chan Result
	done    chan struct{}
	err     error
	msg     *syscall.Msghdr
	buffer  *bytebuffer.Buffer
	br      *BufferAndRing
}

func (in *RecvMultishotInbound) Handle(n int, flags uint32, err error) {
	in.locker.Lock()

	if err != nil {
		if errors.Is(err, syscall.ENOBUFS) { // discard ENOBUFS
			in.locker.Unlock()
			return
		}
		if errors.Is(err, syscall.ECANCELED) { // done
			in.done <- struct{}{}
		}
		in.err = err
		if in.waiting {
			in.waiting = false
			in.ch <- Result{}
		}
		in.locker.Unlock()
		return
	}

	if flags&liburing.IORING_CQE_F_MORE == 0 { // done
		in.done <- struct{}{}
		err = io.EOF
		if in.waiting {
			in.waiting = false
			in.ch <- Result{}
		}
		in.locker.Unlock()
		return
	}

	if _, err = in.br.WriteTo(n, flags, in.buffer); err != nil {
		in.err = err
		if in.waiting {
			in.waiting = false
			in.ch <- Result{}
		}
		in.locker.Unlock()
		return
	}
	if in.waiting {
		in.waiting = false
		in.ch <- Result{}
	}
	in.locker.Unlock()
	return
}

func (in *RecvMultishotInbound) Read(b []byte) (n int, err error) {
	bLen := len(b)
	if bLen == 0 {
		return 0, nil
	}

	in.locker.Lock()
	// try read
	n, _ = in.buffer.Read(b)
	if n == bLen || in.err != nil { // when err exist, means op of ring was finished
		if n == 0 {
			err = in.err
		}
		in.locker.Unlock()
		return
	}

	// read no full, try to wait more
	if n < bLen {
		in.waiting = true
		in.locker.Unlock()
		select {
		case <-in.ch:
			in.locker.Lock()
			nn, _ := in.buffer.Read(b[n:])
			n += nn
			in.locker.Unlock()
			break
		default:
			in.locker.Lock()
			in.waiting = false
			in.locker.Unlock()
			break
		}
		return
	}
	// read nothing, wait more
	in.waiting = true
	in.locker.Unlock()

	<-in.ch
	in.locker.Lock()
	n, _ = in.buffer.Read(b)
	if in.err != nil {
		if n == 0 {
			err = in.err
		}
		in.locker.Unlock()
		return
	}
	in.locker.Unlock()
	return
}

func (in *RecvMultishotInbound) reset() {
	in.waiting = false
	in.ch = nil
	in.buffer = nil
	in.br = nil
	in.msg = nil
	in.err = nil
}

func (in *RecvMultishotInbound) waitDone() {
	<-in.done
}

func newReceiveFuture(fd *Conn) (err error) {
	f := &receiveFuture{
		fd: fd,
	}
	if err = f.prepare(); err != nil {
		return
	}
	if err = f.submit(); err != nil {
		return
	}
	fd.recvFuture = f
	return
}

type receiveFuture struct {
	fd *Conn
	op *Operation
	in *RecvMultishotInbound
	br *BufferAndRing
}

func (f *receiveFuture) prepare() (err error) {
	br, brErr := f.fd.vortex.bufferAndRings.Acquire()
	if brErr != nil {
		err = brErr
		return
	}

	f.op = f.fd.vortex.acquireOperation()
	f.op.Hijack()

	f.in = f.fd.vortex.acquireRecvMultishotInbound(f.op, br, nil)

	f.op.PrepareReceiveMultishot(f.fd, f.in)
	return
}

func (f *receiveFuture) clean() {
	if f.op != nil {
		var (
			op = f.op
			in = f.in
			br = in.br
		)
		f.op = nil
		f.in = nil
		// release op
		op.Complete()
		f.fd.vortex.releaseOperation(op)
		// release br
		f.fd.vortex.bufferAndRings.Release(br)
		// release in
		f.fd.vortex.releaseRecvMultishotInbound(in)
	}
	return
}

func (f *receiveFuture) submit() (err error) {
	if ok := f.fd.vortex.submit(f.op); !ok {
		f.clean()
		err = ErrCancelled
		return
	}
	return
}

func (f *receiveFuture) receive(b []byte) (n int, err error) {
RETRY:
	n, err = f.in.Read(b)
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = nil
			return
		}
		if errors.Is(err, ErrIOURingSQBusy) { // not submitted, try to submit again
			if err = f.submit(); err != nil {
				return
			}
			goto RETRY
		}
	}
	return
}

func (f *receiveFuture) Cancel() (err error) {
	if f.op != nil {
		err = f.fd.vortex.cancelOperation(f.op)
		f.in.waitDone()
		f.clean()
	}
	return
}
