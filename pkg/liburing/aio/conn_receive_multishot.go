//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/bytebuffer"
	"io"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func newRecvMultishotHandler(conn *Conn) (handler *RecvMultishotHandler, err error) {
	// br
	br, brErr := conn.eventLoop.AcquireBufferAndRing()
	if brErr != nil {
		err = brErr
		return
	}
	// buffer
	buffer := bytebuffer.Acquire()
	// op
	op := conn.eventLoop.resource.AcquireOperation()
	op.Hijack()
	// handler
	handler = &RecvMultishotHandler{
		conn:    conn,
		op:      op,
		locker:  new(sync.Mutex),
		waiting: new(atomic.Bool),
		err:     nil,
		buffer:  buffer,
		br:      br,
		ch:      make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	// prepare
	op.PrepareReceiveMultishot(conn, br, handler)
	// submit
	if err = handler.submit(); err != nil {
		// release op
		op.Complete()
		conn.eventLoop.resource.ReleaseOperation(op)
		// release br
		conn.eventLoop.ReleaseBufferAndRing(br)
		// release buffer
		bytebuffer.Release(buffer)
	}
	return
}

type RecvMultishotHandler struct {
	conn    *Conn
	op      *Operation
	locker  sync.Locker
	waiting *atomic.Bool
	err     error
	buffer  *bytebuffer.Buffer
	br      *BufferAndRing
	ch      chan struct{}
	done    chan struct{}
}

func (handler *RecvMultishotHandler) Handle(n int, flags uint32, err error) {
	// handle ERR
	if err != nil {
		//fmt.Println("RECV > ", handler.conn.Name(),
		//	n, err,
		//	"CQE_F_MORE", flags&liburing.IORING_CQE_F_MORE,
		//	"CQE_F_SOCK_NONEMPTY", flags&liburing.IORING_CQE_F_SOCK_NONEMPTY,
		//	"CQE_F_BUFFER", flags&liburing.IORING_CQE_F_BUFFER,
		//)
		if errors.Is(err, syscall.ENOBUFS) { // try to submit again
			if err = handler.submit(); err != nil {
				handler.locker.Lock()
				handler.err = err
				handler.locker.Unlock()
				close(handler.done)
			}
			return
		}
		// done when err happened
		handler.locker.Lock()
		handler.err = err
		handler.locker.Unlock()

		if handler.waiting.CompareAndSwap(true, false) {
			handler.ch <- struct{}{}
		}

		close(handler.done)
		return
	}

	// handle CQE_F_BUFFER
	if flags&liburing.IORING_CQE_F_BUFFER != 0 {
		handler.locker.Lock()
		if _, err = handler.br.WriteTo(n, flags, handler.buffer); err != nil {
			handler.err = err
		}
		handler.locker.Unlock()

		if handler.waiting.CompareAndSwap(true, false) {
			handler.ch <- struct{}{}
		}
	}

	// handle CQE_F_MORE
	if flags&liburing.IORING_CQE_F_MORE == 0 {
		//fmt.Println("RECV > ", handler.conn.Name(),
		//	n, err,
		//	"CQE_F_MORE", flags&liburing.IORING_CQE_F_MORE,
		//	"CQE_F_SOCK_NONEMPTY", flags&liburing.IORING_CQE_F_SOCK_NONEMPTY,
		//	"CQE_F_BUFFER", flags&liburing.IORING_CQE_F_BUFFER,
		//)
		if n == 0 { // EOF
			handler.locker.Lock()
			handler.err = io.EOF
			handler.locker.Unlock()

			if handler.waiting.CompareAndSwap(true, false) {
				handler.ch <- struct{}{}
			}

			close(handler.done)
		} else { // has more data but operation was canceled, to try to submit again
			if err = handler.submit(); err != nil {
				handler.locker.Lock()
				handler.err = err
				handler.locker.Unlock()
				close(handler.done)
			}
		}
		return
	}

	return
}

func (handler *RecvMultishotHandler) Receive(b []byte) (n int, err error) {
	bLen := len(b)
	if bLen == 0 {
		return 0, nil
	}

	handler.locker.Lock()

	// try read
	n, _ = handler.buffer.Read(b)
	if n == bLen { // read full
		handler.locker.Unlock()
		return
	}
	// handler err
	if handler.err != nil {
		if n == 0 {
			err = handler.err
			if errors.Is(err, io.EOF) && !handler.conn.zeroReadIsEOF {
				err = nil
			}
		}
		handler.locker.Unlock()
		return
	}
	// mark waiting more
	handler.waiting.Store(true)
	handler.locker.Unlock()

	// try read more when read not full
	if 0 < n && n < bLen {
		select {
		case <-handler.ch:
			handler.locker.Lock()
			nn, _ := handler.buffer.Read(b[n:])
			n += nn
			handler.locker.Unlock()
			break
		default:
			if !handler.waiting.CompareAndSwap(true, false) { // means written
				<-handler.ch
				handler.locker.Lock()
				nn, _ := handler.buffer.Read(b[n:])
				n += nn
				handler.locker.Unlock()
			}
			break
		}
		return
	}
	// wait
	var timer *time.Timer
	if !handler.conn.readDeadline.IsZero() {
		timeout := time.Until(handler.conn.readDeadline)
		if timeout < 1 {
			err = ErrTimeout
			return
		}
		timer = handler.conn.eventLoop.resource.AcquireTimer(timeout)
		defer handler.conn.eventLoop.resource.ReleaseTimer(timer)
	}

	if timer == nil {
		<-handler.ch
		handler.locker.Lock()
		n, _ = handler.buffer.Read(b)
		handler.locker.Unlock()

		if n == 0 && handler.err != nil {
			err = handler.err
			if errors.Is(err, io.EOF) && !handler.conn.zeroReadIsEOF {
				err = nil
			}
		}
	} else {
		select {
		case <-handler.ch:
			handler.locker.Lock()
			n, _ = handler.buffer.Read(b)
			handler.locker.Unlock()
			if n == 0 && handler.err != nil {
				err = handler.err
				if errors.Is(err, io.EOF) && !handler.conn.zeroReadIsEOF {
					err = nil
				}
			}
			break
		case <-timer.C:
			err = ErrTimeout
			break
		}
	}

	return
}

func (handler *RecvMultishotHandler) Close() (err error) {
	handler.locker.Lock()
	if handler.op == nil {
		handler.locker.Unlock()
		return
	}
	if errors.Is(handler.err, io.EOF) {
		handler.locker.Unlock()
		<-handler.done
		handler.clean()
		return
	}
	handler.locker.Unlock()

	op := handler.op
	if err = handler.conn.eventLoop.Cancel(op); err != nil {
		handler.locker.Lock()
		if !errors.Is(handler.err, io.EOF) {
			handler.locker.Unlock()
			// use cancel fd when cancel op failed
			handler.conn.Cancel()
		} else {
			handler.locker.Unlock()
		}
		// reset err when fd was canceled
		err = nil
	}
	// wait done to clean
	<-handler.done
	handler.clean()
	return
}

func (handler *RecvMultishotHandler) submit() (err error) {
	handler.locker.Lock()
	defer handler.locker.Unlock()
	if handler.op == nil {
		err = ErrCanceled
		return
	}
	if err = handler.conn.eventLoop.Submit(handler.op); err != nil {
		return
	}
	return
}

func (handler *RecvMultishotHandler) clean() {
	handler.locker.Lock()
	if op := handler.op; op != nil {
		// release op
		handler.op = nil
		op.Complete()
		handler.conn.eventLoop.resource.ReleaseOperation(op)
		// release br
		br := handler.br
		handler.br = nil
		handler.conn.eventLoop.ReleaseBufferAndRing(br)
		// release buffer
		buffer := handler.buffer
		handler.buffer = nil
		bytebuffer.Release(buffer)
		// close ch
		close(handler.ch)
	}
	handler.locker.Unlock()
	return
}
