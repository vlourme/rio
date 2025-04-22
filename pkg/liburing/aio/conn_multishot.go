//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/bytebuffer"
	"io"
	"net"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type MultishotReceiveAdaptor struct {
	br *BufferAndRing
}

func (adaptor *MultishotReceiveAdaptor) Handle(n int, flags uint32, err error) (bool, int, uint32, unsafe.Pointer, error) {
	if err != nil || flags&liburing.IORING_CQE_F_BUFFER == 0 {
		return true, n, flags, nil, err
	}

	var (
		br   = adaptor.br
		bid  = uint16(flags >> liburing.IORING_CQE_BUFFER_SHIFT)
		beg  = int(bid) * br.config.Size
		end  = beg + br.config.Size
		mask = br.config.mask
	)
	if n == 0 {
		b := br.buffer[beg:end]
		br.value.BufRingAdd(unsafe.Pointer(&b[0]), uint32(br.config.Size), bid, mask, 0)
		br.value.BufRingAdvance(1)
		return true, n, flags, nil, nil
	}
	buf := bytebuffer.Acquire()
	length := n
	for length > 0 {
		if br.config.Size > length {
			_, _ = buf.Write(br.buffer[beg : beg+length])

			b := br.buffer[beg:end]
			br.value.BufRingAdd(unsafe.Pointer(&b[0]), uint32(br.config.Size), bid, mask, 0)
			br.value.BufRingAdvance(1)
			break
		}

		_, _ = buf.Write(br.buffer[beg:end])

		b := br.buffer[beg:end]
		br.value.BufRingAdd(unsafe.Pointer(&b[0]), uint32(br.config.Size), bid, mask, 0)
		br.value.BufRingAdvance(1)

		length -= br.config.Size
		bid = (bid + 1) % uint16(br.config.Count)
		beg = int(bid) * br.config.Size
		end = beg + br.config.Size
	}

	return true, n, flags, unsafe.Pointer(buf), nil
}

func (adaptor *MultishotReceiveAdaptor) HandleCompletionEvent(event CompletionEvent, b []byte, overflow io.Writer) (n int, interrupted bool, err error) {
	// handle error
	if event.Err != nil {
		err = event.Err
		return
	}

	// handle attachment
	if attachment := event.Attachment; attachment != nil {
		buf := (*bytebuffer.Buffer)(attachment)
		n, _ = buf.Read(b)
		if buf.Len() > 0 {
			_, _ = buf.WriteTo(overflow)
		}
		event.Attachment = nil
		bytebuffer.Release(buf)
	}

	// handle IORING_CQE_F_MORE is 0
	if event.Flags&liburing.IORING_CQE_F_MORE == 0 {
		if event.N == 0 {
			err = io.EOF
			return
		}
		interrupted = true
	}
	return
}

const (
	recvMultishotReady = iota
	recvMultishotProcessing
	recvMultishotCanceled
)

func newMultishotReceiver(conn *Conn) (receiver *MultishotReceiver, err error) {
	// acquire buffer and ring
	br, brErr := conn.eventLoop.AcquireBufferAndRing()
	if brErr != nil {
		err = brErr
		return
	}
	// adaptor
	adaptor := &MultishotReceiveAdaptor{br: br}
	// op
	op := AcquireOperation()
	op.PrepareReceiveMultishot(conn, adaptor)
	// receiver
	receiver = &MultishotReceiver{
		status:        recvMultishotReady,
		locker:        sync.Mutex{},
		operationLock: sync.Mutex{},
		buffer:        bytebuffer.Acquire(),
		adaptor:       adaptor,
		eventLoop:     conn.eventLoop,
		operation:     op,
		future:        nil,
		err:           nil,
	}
	return
}

type MultishotReceiver struct {
	status        int
	locker        sync.Mutex
	operationLock sync.Mutex
	adaptor       *MultishotReceiveAdaptor
	buffer        *bytebuffer.Buffer
	eventLoop     *EventLoop
	operation     *Operation
	future        Future
	err           error
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

	// read buffer
	if r.buffer.Len() > 0 {
		n, _ = r.buffer.Read(b)
		if n == bLen {
			r.locker.Unlock()
			return
		}
	}

	// start
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
		nn, interrupted, handleErr := r.adaptor.HandleCompletionEvent(event, b[n:], r.buffer)
		n += nn
		if handleErr != nil {
			if errors.Is(handleErr, syscall.ENOBUFS) { // set ready to resubmit next receive time
				r.status = recvMultishotReady
				break
			}

			r.status = recvMultishotCanceled // set done when receive failed
			r.err = handleErr

			if IsTimeout(handleErr) { // timeout is not iouring err, means op is alive, then cancel it
				if r.cancel() {
					for { // await cancel
						_, _, _, err = r.future.Await()
						if IsCanceled(err) { // op canceled
							err = nil
							break
						}
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
	r.operationLock.Lock()
	if op := r.operation; op != nil {
		ok := r.eventLoop.Cancel(r.operation) == nil
		r.operationLock.Unlock()
		return ok
	}
	r.operationLock.Unlock()
	return false
}

func (r *MultishotReceiver) releaseRuntime() {
	// release op
	if op := r.operation; op != nil {
		r.operation = nil
		r.future = nil
		ReleaseOperation(op)
	}
	// release br
	if adaptor := r.adaptor; adaptor != nil {
		br := adaptor.br
		adaptor.br = nil
		r.adaptor = nil
		r.eventLoop.ReleaseBufferAndRing(br)
	}
}

func (r *MultishotReceiver) releaseBuffer() {
	if buffer := r.buffer; buffer != nil {
		r.buffer = nil
		buffer.Reset()
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
			}
			r.releaseRuntime()
			r.releaseBuffer()
			r.status = recvMultishotCanceled
			r.err = ErrCanceled
			break
		case recvMultishotCanceled: // canceled, then pass
			break
		default:
			r.locker.Unlock()
			panic("unreachable multishot receiver status")
			return
		}
		r.locker.Unlock()
		return
	}
	// unlocked means reading, then cancel it
	r.cancel()
	return
}

type MultishotMsgReceiveAdaptor struct {
	br  *BufferAndRing
	msg *syscall.Msghdr
}

func (r *MultishotMsgReceiveAdaptor) Handle(n int, flags uint32, err error) (bool, int, uint32, unsafe.Pointer, error) {
	//TODO implement me
	panic("implement me")
}

type MultishotMsgReceiver struct {
	status        int
	locker        *sync.Mutex
	adaptor       *MultishotMsgReceiveAdaptor
	eventLoop     *EventLoop
	operation     *Operation
	operationLock *sync.Mutex
	future        Future
	err           error
}

func (r *MultishotMsgReceiver) ReceiveMsg(b []byte, oob []byte, flags int) (n int, oobn int, flag int, addr net.Addr, err error) {
	//TODO implement me
	panic("implement me")
}

func (r *MultishotMsgReceiver) Close() (err error) {

	return
}
