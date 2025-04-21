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
	recvMultishotCanceled
)

type MultishotReceiver struct {
	status    int
	locker    *sync.Mutex
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
	// handle canceled
	if r.status == recvMultishotCanceled {
		if r.buffer == nil {
			err = r.err
			r.locker.Unlock()
			return
		}
		n, _ = r.buffer.Read(b)
		if n == 0 {
			r.releaseBuffer()
			err = r.err
		}
		r.locker.Unlock()
		return
	}
	r.locker.Unlock()

	// read buffer
	if r.buffer.Len() > 0 {
		n, _ = r.buffer.Read(b)
		if n == bLen {
			return
		}
	}

	// start
	r.locker.Lock()
	if r.status == recvMultishotCanceled {
		if n == 0 {
			r.releaseBuffer()
			err = r.err
		}
		r.locker.Unlock()
		return
	}
	if r.status == recvMultishotReady {
		r.submit()
		r.status = recvMultishotProcessing
	}

	// await
	hungry := n == 0 // when n > 0, then try await
	events := r.future.AwaitBatch(hungry, deadline)
	// handle events
	eventsLen := len(events)
	if eventsLen == 0 { // nothing received
		r.locker.Unlock()
		return
	}

	// note: when event contains err, means it is the last in events, so break loop is ok
	for i := 0; i < eventsLen; i++ {
		event := events[i]
		nn, interrupted, handleErr := r.br.HandleCompletionEvent(event, b[n:], r.buffer)
		n += nn
		if handleErr != nil {
			if errors.Is(handleErr, syscall.ENOBUFS) { // set ready to resubmit next receive time
				r.status = recvMultishotReady
				break
			}

			r.status = recvMultishotCanceled // set done when receive failed
			r.err = handleErr

			if IsTimeout(handleErr) { // handle timeout
				r.cancel()
				for { // await cancel
					_, _, _, err = r.future.Await()
					if IsCanceled(err) { // op canceled
						err = nil
						break
					}
				}
			}
			r.releaseRuntime() // release runtime

			if n == 0 {
				r.releaseBuffer()
				err = handleErr
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
}

func (r *MultishotReceiver) cancel() bool {
	return r.eventLoop.Cancel(r.operation) == nil
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
	if r.locker.TryLock() { // locked means no reader lock the mr
		switch r.status {
		case recvMultishotReady: // un submitted, then mark canceled.
			r.releaseRuntime()
			r.releaseBuffer()

			r.status = recvMultishotCanceled
			r.err = ErrCanceled
			break
		case recvMultishotProcessing: // cancel and handle events
			if r.cancel() {
				for {
					_, _, _, err = r.future.Await()
					if IsCanceled(err) { // op canceled
						err = nil
						break
					}
				}
				r.releaseRuntime()
				r.releaseBuffer()
			}
			r.status = recvMultishotCanceled
			r.err = ErrCanceled
			break
		case recvMultishotCanceled:
			break
		default:
			r.locker.Unlock()
			panic("unreachable multishot receiver status")
			return
		}
		r.locker.Unlock()
		return
	}
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

func OOBLen() int {
	return syscall.CmsgLen(syscall.SizeofSockaddrAny) + syscall.SizeofCmsghdr
}
