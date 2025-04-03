//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/bytebuffer"
	"io"
	"sync"
	"syscall"
	"time"
)

func newRecvMultishotHandler(conn *Conn) (handler *RecvMultishotHandler, err error) {
	// br
	br, brErr := conn.vortex.bufferAndRings.Acquire()
	if brErr != nil {
		err = brErr
		return
	}
	// buffer
	buffer := bytebuffer.Acquire()
	// op
	op := conn.vortex.acquireOperation()
	op.Hijack()
	// handler
	handler = &RecvMultishotHandler{
		conn:    conn,
		op:      op,
		locker:  sync.Mutex{},
		waiting: false,
		err:     nil,
		buffer:  buffer,
		br:      br,
		ch:      op.resultCh,
	}
	// prepare
	op.PrepareReceiveMultishot(conn, br, handler)
	// submit
	if err = handler.submit(); err != nil {
		// release op
		op.Complete()
		conn.vortex.releaseOperation(op)
		// release br
		conn.vortex.bufferAndRings.Release(br)
		// release buffer
		bytebuffer.Release(buffer)
	}
	return
}

type RecvMultishotHandler struct {
	conn    *Conn
	op      *Operation
	locker  sync.Mutex
	waiting bool
	err     error
	buffer  *bytebuffer.Buffer
	br      *BufferAndRing
	ch      chan Result
}

func (handler *RecvMultishotHandler) Handle(n int, flags uint32, err error) {
	handler.locker.Lock()

	if err != nil {
		if errors.Is(err, syscall.ENOBUFS) { // discard ENOBUFS
			handler.locker.Unlock()
			return
		}
		handler.err = err
		if handler.waiting {
			handler.waiting = false
			handler.ch <- Result{}
		}
		handler.locker.Unlock()
		return
	}

	if flags&liburing.IORING_CQE_F_MORE == 0 { // EOF
		handler.err = io.EOF
		if handler.waiting {
			handler.waiting = false
			handler.ch <- Result{}
		}
		handler.locker.Unlock()
		return
	}

	if _, err = handler.br.WriteTo(n, flags, handler.buffer); err != nil {
		handler.err = err
	}

	if handler.waiting {
		handler.waiting = false
		handler.ch <- Result{}
	}

	handler.locker.Unlock()
	return
}

func (handler *RecvMultishotHandler) Receive(b []byte) (n int, err error) {
	bLen := len(b)
	if bLen == 0 {
		return 0, nil
	}

	handler.locker.Lock()
	// check sq busy error
	if handler.err != nil && errors.Is(handler.err, ErrIOURingSQBusy) {
		if err = handler.submit(); err != nil {
			handler.err = err
			handler.locker.Unlock()
			return
		}
	}

	// try read
	n, _ = handler.buffer.Read(b)
	if n == bLen { // read full
		handler.locker.Unlock()
		return
	}
	// handler err
	if handler.err != nil {
		handler.locker.Unlock()
		if n == 0 {
			handler.clean()
			err = handler.err
			if errors.Is(err, io.EOF) && !handler.conn.zeroReadIsEOF {
				err = nil
			}
		}
		return
	}
	// mark waiting more
	handler.waiting = true
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
			handler.locker.Lock()
			handler.waiting = false
			handler.locker.Unlock()
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
		timer = handler.conn.vortex.acquireTimer(timeout)
		defer handler.conn.vortex.releaseTimer(timer)
	}

	if timer == nil {
		<-handler.ch
		handler.locker.Lock()
		n, _ = handler.buffer.Read(b)
		handler.locker.Unlock()
		if n == 0 && handler.err != nil {
			handler.clean()
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
				handler.clean()
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
	op := handler.op
	if err = handler.conn.vortex.cancelOperation(op); err == nil {
		handler.locker.Unlock()
		handler.clean()
		return
	}
	handler.locker.Unlock()
	// use cancel fd when cancel op failed
	handler.conn.Cancel()
	handler.clean()
	return
}

func (handler *RecvMultishotHandler) submit() (err error) {
	if ok := handler.conn.vortex.submit(handler.op); !ok {
		err = ErrCanceled
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
		handler.conn.vortex.releaseOperation(op)
		// release br
		br := handler.br
		handler.br = nil
		handler.conn.vortex.bufferAndRings.Release(br)
		// release buffer
		buffer := handler.buffer
		handler.buffer = nil
		bytebuffer.Release(buffer)
	}
	handler.locker.Unlock()
	return
}
