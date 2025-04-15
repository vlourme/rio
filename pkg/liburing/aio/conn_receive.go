//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing/aio/bytebuffer"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

func (c *Conn) Receive(b []byte) (n int, err error) {
	if c.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}
	if c.multishot {
		if c.multishotReceiver == nil {
			c.multishotReceiver, err = newMultishotReceiver(c)
			if err != nil {
				c.multishot = false
				err = nil
				n, err = c.receiveOneshot(b)
				return
			}
		}
		n, err = c.multishotReceiver.Recv(b, c.readDeadline)
		if err != nil && errors.Is(err, io.EOF) {
			if !c.zeroReadIsEOF {
				err = nil
			}
		}
	} else {
		n, err = c.receiveOneshot(b)
	}
	return
}

func (c *Conn) receiveOneshot(b []byte) (n int, err error) {
	op := AcquireOperationWithDeadline(c.readDeadline)
	op.PrepareReceive(c, b)
	n, _, err = c.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
	if n == 0 && err == nil && c.zeroReadIsEOF {
		err = io.EOF
	}
	return
}

func newMultishotReceiver(conn *Conn) (receiver *MultishotReceiver, err error) {
	// acquire buffer and ring
	br, brErr := conn.eventLoop.AcquireBufferAndRing()
	if brErr != nil {
		err = brErr
		return
	}
	// acquire op
	op := AcquireOperation()
	op.PrepareReceiveMultishot(conn, br)

	receiver = &MultishotReceiver{
		status:    recvMultishotReady,
		locker:    new(sync.Mutex),
		buffer:    bytebuffer.Acquire(),
		br:        br,
		eventLoop: conn.eventLoop,
		operation: op,
		future:    nil,
		err:       nil,
	}
	return
}

const (
	recvMultishotReady = iota
	recvMultishotProcessing
	recvMultishotEOF
	recvMultishotCanceled
)

type MultishotReceiver struct {
	status    int
	locker    sync.Locker
	buffer    *bytebuffer.Buffer
	br        *BufferAndRing
	eventLoop *EventLoop
	operation *Operation
	future    Future
	err       error
}

func (r *MultishotReceiver) Recv(b []byte, deadline time.Time) (n int, err error) {
	bLen := len(b)
	if bLen == 0 {
		return
	}

	r.locker.Lock()
	// canceled
	if r.status == recvMultishotCanceled {
		err = r.err
		r.releaseBuffer()
		r.locker.Unlock()
		return
	}
	// read buffer
	if r.buffer.Len() > 0 {
		n, _ = r.buffer.Read(b)
		if n == bLen {
			r.locker.Unlock()
			return
		}
	}
	// eof
	if r.status == recvMultishotEOF {
		if n == 0 {
			err = io.EOF
		}
		r.releaseBuffer()
		r.locker.Unlock()
		return
	}

	// start
	if r.status == recvMultishotReady {
		r.submit()
	}
	// await
	hungry := n == 0 // when n > 0, then try await
	events := r.future.AwaitBatch(hungry, deadline)
	eventsLen := len(events)
	if eventsLen == 0 { // nothing to read
		r.locker.Unlock()
		return
	}
	// when event contains err, means it is the last in events, so break loop is ok
	for i := 0; i < eventsLen; i++ {
		event := events[i]
		nn, interrupted, handleErr := r.br.HandleCompletionEvent(event, b[n:], r.buffer)
		n += nn
		if handleErr != nil {
			if errors.Is(handleErr, syscall.ENOBUFS) { // set ready to resubmit next read time
				r.status = recvMultishotReady
				break
			}

			r.status = recvMultishotCanceled // set done when read failed
			r.err = handleErr

			if IsTimeout(handleErr) { // timeout then cancel
				r.cancel()
			}

			if errors.Is(handleErr, io.EOF) { // set EOF
				r.status = recvMultishotEOF
			}

			r.releaseRuntime() // release runtime

			if n == 0 {
				err = handleErr
				r.releaseBuffer() // release buffer
			}
			break
		}
		if interrupted { // set ready to resubmit next read time
			r.status = recvMultishotReady
			break
		}
	}
	r.locker.Unlock()
	return
}

func (r *MultishotReceiver) submit() {
	r.future = r.eventLoop.Submit(r.operation)
	r.status = recvMultishotProcessing
}

func (r *MultishotReceiver) cancel() {
	if err := r.eventLoop.Cancel(r.operation); err == nil {
		for {
			_, _, _, err = r.future.Await()
			if IsCanceled(err) { // op canceled
				err = nil
				break
			}
		}
	}
	return
}

func (r *MultishotReceiver) releaseRuntime() {
	// release op
	if op := r.operation; op != nil {
		r.operation = nil
		r.future = nil
		ReleaseOperation(op)
	}
	// release br
	if br := r.br; br != nil {
		r.br = nil
		r.eventLoop.ReleaseBufferAndRing(br)
	}
}

func (r *MultishotReceiver) releaseBuffer() {
	if buffer := r.buffer; buffer != nil {
		r.buffer = nil
		bytebuffer.Release(buffer)
	}
}

func (r *MultishotReceiver) Close() (err error) {
	r.locker.Lock()
	if r.status == recvMultishotProcessing {
		r.cancel()
		r.status = recvMultishotCanceled
		r.err = ErrCanceled
		r.releaseRuntime()
		r.releaseBuffer()
	}
	r.locker.Unlock()
	return
}

func (c *Conn) ReceiveFrom(b []byte) (n int, addr net.Addr, err error) {
	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	msg := acquireMsg(b, nil, rsa, rsaLen, 0)

	op := AcquireOperationWithDeadline(c.readDeadline)
	op.PrepareReceiveMsg(c, msg)
	n, _, err = c.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)

	releaseMsg(msg)
	if err != nil {
		return
	}
	sa, saErr := sys.RawSockaddrAnyToSockaddr(rsa)
	if saErr != nil {
		err = saErr
		return
	}
	addr = sys.SockaddrToAddr(c.Net(), sa)
	return
}

func (c *Conn) ReceiveMsg(b []byte, oob []byte, flags int) (n int, oobn int, flag int, addr net.Addr, err error) {
	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	msg := acquireMsg(b, oob, rsa, rsaLen, int32(flags))

	op := AcquireOperationWithDeadline(c.readDeadline)
	op.PrepareReceiveMsg(c, msg)
	n, _, err = c.eventLoop.SubmitAndWait(op)
	if err == nil {
		oobn = int(msg.Controllen)
		flag = int(msg.Flags)
	}
	ReleaseOperation(op)

	releaseMsg(msg)

	if err != nil {
		return
	}
	sa, saErr := sys.RawSockaddrAnyToSockaddr(rsa)
	if saErr != nil {
		err = saErr
		return
	}
	addr = sys.SockaddrToAddr(c.Net(), sa)
	return
}
