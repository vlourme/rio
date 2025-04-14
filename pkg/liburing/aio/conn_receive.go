//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/bytebuffer"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"io"
	"net"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

func (c *Conn) Receive(b []byte) (n int, err error) {
	if c.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}
	c.locker.Lock()
	if c.multishot {
		if c.recvMultishotAdaptor == nil {
			c.recvMultishotAdaptor, err = acquireRecvMultishotAdaptor(c)
			if err != nil {
				c.multishot = false
				err = nil
				n, err = c.receiveOneshot(b)
				c.locker.Unlock()
				return
			}
		}
		n, err = c.recvMultishotAdaptor.Read(b, c.readDeadline)
		if err != nil && errors.Is(err, io.EOF) {
			if !c.zeroReadIsEOF {
				err = nil
			}
		}
	} else {
		n, err = c.receiveOneshot(b)
	}
	c.locker.Unlock()
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

var (
	recvMultishotAdaptors = sync.Pool{}
)

func acquireRecvMultishotAdaptor(conn *Conn) (adaptor *RecvMultishotAdaptor, err error) {
	br, brErr := conn.eventLoop.AcquireBufferAndRing()
	if brErr != nil {
		err = brErr
		return
	}
	v := recvMultishotAdaptors.Get()
	if v == nil {
		adaptor = &RecvMultishotAdaptor{
			locker: new(sync.Mutex),
			status: recvMultishotReady,
			wg:     new(sync.WaitGroup),
		}
	} else {
		adaptor = v.(*RecvMultishotAdaptor)
	}
	adaptor.br = br
	adaptor.eventLoop = conn.eventLoop
	adaptor.operation = AcquireOperation()
	adaptor.buffer = bytebuffer.Acquire()

	adaptor.operation.PrepareReceiveMultishot(conn, adaptor)
	return
}

func releaseRecvMultishotAdaptor(adaptor *RecvMultishotAdaptor) {
	if adaptor == nil {
		return
	}
	recvMultishotAdaptors.Put(adaptor)
}

const (
	recvMultishotReady = iota
	recvMultishotProcessing
	recvMultishotEOF
	recvMultishotCanceled
)

type RecvMultishotAdaptor struct {
	locker    sync.Locker
	status    int
	wg        *sync.WaitGroup
	buffer    *bytebuffer.Buffer
	br        *BufferAndRing
	eventLoop *EventLoop
	operation *Operation
	future    Future
	err       error
}

func (adaptor *RecvMultishotAdaptor) Read(b []byte, deadline time.Time) (n int, err error) {
	bLen := len(b)
	if bLen == 0 {
		return
	}

	if adaptor.buffer.Len() > 0 {
		n, _ = adaptor.buffer.Read(b)
		if n == bLen {
			return
		}
	}

	adaptor.locker.Lock()

	// eof
	if adaptor.status == recvMultishotEOF {
		if n == 0 {
			err = io.EOF
		}
		adaptor.locker.Unlock()
		return
	}
	// canceled
	if adaptor.status == recvMultishotCanceled {
		if n == 0 {
			err = adaptor.err
		}
		adaptor.locker.Unlock()
		return
	}
	adaptor.wg.Add(1)
	// start
	if adaptor.status == recvMultishotReady {
		adaptor.future = adaptor.eventLoop.Submit(adaptor.operation)
		adaptor.status = recvMultishotProcessing
	}
	// await
	hungry := n == 0
	events := adaptor.future.AwaitMore(hungry, deadline)
	eventsLen := len(events)
	if eventsLen == 0 { // nothing to read
		adaptor.wg.Done()
		adaptor.locker.Unlock()
		return
	}
	// when event contains err, means it is the last in events, so break loop is ok
	for i := 0; i < eventsLen; i++ {
		event := events[i]
		// handle err
		if event.Err != nil {
			if errors.Is(event.Err, syscall.ENOBUFS) { // set ready to resubmit next read time
				adaptor.status = recvMultishotReady
				break
			}

			adaptor.status = recvMultishotCanceled // set done when read failed
			adaptor.err = event.Err
			if n == 0 {
				err = event.Err
			}
			break
		}

		// handle attachment
		if event.Attachment != nil {
			buf := (*bytebuffer.Buffer)(event.Attachment)
			event.Attachment = nil
			nn, _ := buf.Read(b[n:])
			n += nn
			if buf.Len() > 0 {
				_, _ = buf.WriteTo(adaptor.buffer)
			}
			bytebuffer.Release(buf)
		}
		// handle CQE_F_MORE
		if event.Flags&liburing.IORING_CQE_F_MORE == 0 {
			if event.N == 0 { // eof then set done
				adaptor.status = recvMultishotEOF
				if n == 0 {
					err = io.EOF
				}
				break
			}
			// maybe has more content, then set ready
			adaptor.status = recvMultishotReady
			break
		}
	}
	adaptor.wg.Done()
	adaptor.locker.Unlock()
	return
}

func (adaptor *RecvMultishotAdaptor) Handle(n int, flags uint32, err error) (bool, int, uint32, unsafe.Pointer, error) {
	if err != nil || flags&liburing.IORING_CQE_F_BUFFER == 0 {
		return true, n, flags, nil, err
	}

	var (
		bid  = uint16(flags >> liburing.IORING_CQE_BUFFER_SHIFT)
		beg  = int(bid) * adaptor.br.config.Size
		end  = beg + adaptor.br.config.Size
		mask = adaptor.br.config.mask
	)
	if n == 0 {
		b := adaptor.br.buffer[beg:end]
		adaptor.br.value.BufRingAdd(unsafe.Pointer(&b[0]), uint32(adaptor.br.config.Size), bid, mask, 0)
		adaptor.br.value.BufRingAdvance(1)
		return true, n, flags, nil, nil
	}
	buf := bytebuffer.Acquire()
	length := n
	for length > 0 {
		if adaptor.br.config.Size > length {
			_, _ = buf.Write(adaptor.br.buffer[beg : beg+length])

			b := adaptor.br.buffer[beg:end]
			adaptor.br.value.BufRingAdd(unsafe.Pointer(&b[0]), uint32(adaptor.br.config.Size), bid, mask, 0)
			adaptor.br.value.BufRingAdvance(1)
			break
		}

		_, _ = buf.Write(adaptor.br.buffer[beg:end])

		b := adaptor.br.buffer[beg:end]
		adaptor.br.value.BufRingAdd(unsafe.Pointer(&b[0]), uint32(adaptor.br.config.Size), bid, mask, 0)
		adaptor.br.value.BufRingAdvance(1)

		length -= adaptor.br.config.Size
		bid = (bid + 1) % uint16(adaptor.br.config.Count)
		beg = int(bid) * adaptor.br.config.Size
		end = beg + adaptor.br.config.Size
	}

	return true, n, flags, unsafe.Pointer(buf), nil
}

func (adaptor *RecvMultishotAdaptor) Close() (err error) {
	adaptor.locker.Lock()
	if adaptor.status == recvMultishotProcessing {
		if err = adaptor.eventLoop.Cancel(adaptor.operation); err == nil { // cancel succeed
			adaptor.locker.Unlock()
			adaptor.wg.Wait()
		} else {
			adaptor.locker.Unlock()
		}
	}

	br := adaptor.br
	adaptor.br = nil
	adaptor.eventLoop.ReleaseBufferAndRing(br)
	adaptor.status = recvMultishotReady
	adaptor.eventLoop = nil
	adaptor.operation = nil
	adaptor.future = nil
	buffer := adaptor.buffer
	adaptor.buffer = nil
	bytebuffer.Release(buffer)

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
