//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	_supportListen    bool
	_supportBind      bool
	_supportSendZC    bool
	_supportSendMSGZC bool
	_supportOnce      sync.Once
)

func supportOpCheck() {
	_supportOnce.Do(func() {
		probe, probeErr := liburing.GetProbe()
		if probeErr != nil {
			return
		}
		_supportListen = probe.IsSupported(liburing.IORING_OP_LISTEN)
		_supportBind = probe.IsSupported(liburing.IORING_OP_BIND)
		_supportSendZC = probe.IsSupported(liburing.IORING_OP_SEND_ZC)
		_supportSendMSGZC = probe.IsSupported(liburing.IORING_OP_SENDMSG_ZC)
	})
}

func supportListen() bool {
	supportOpCheck()
	return _supportListen
}

func supportBind() bool {
	supportOpCheck()
	return _supportBind
}

func supportSendZC() bool {
	supportOpCheck()
	return _supportSendZC
}

func supportSendMSGZC() bool {
	supportOpCheck()
	return _supportSendMSGZC
}

type Resource struct {
	operations       sync.Pool
	msgs             sync.Pool
	timers           sync.Pool
	oneshotFutures   sync.Pool
	zeroCopyFutures  sync.Pool
	multishotFutures sync.Pool
}

const (
	OneshotOperation = iota
	ZeroCopyOperation
	MultiShotOperation
)

func (res *Resource) AcquireOperation1(kind int, deadline time.Time) (op *Operation) {
	v := res.operations.Get()
	if v == nil {
		op = &Operation{
			code:  liburing.IORING_OP_LAST,
			flags: op_f_borrowed,
		}
	} else {
		op = v.(*Operation)
	}

	switch kind {
	case OneshotOperation:
		f0 := res.oneshotFutures.Get()
		if f0 == nil {
			f0 = &OneshotFuture{
				baseFuture{ch: make(chan FutureResult, 1)},
			}
		}
		op.promise = f0.(*OneshotFuture)
		break
	case ZeroCopyOperation:
		f0 := res.zeroCopyFutures.Get()
		if f0 == nil {
			f0 = &ZeroCopyFuture{
				baseFuture: baseFuture{ch: make(chan FutureResult, 2)},
				result:     FutureResult{},
			}
		}
		op.promise = f0.(*ZeroCopyFuture)
		break
	case MultiShotOperation:
		f0 := res.multishotFutures.Get()
		if f0 == nil {
			f0 = &MultishotFuture{
				baseFuture: baseFuture{ch: make(chan FutureResult, 4096)},
			}
		}
		op.promise = f0.(*MultishotFuture)
		op.flags |= op_f_multishot
		break
	default:
		panic(errors.New("invalid operation kind"))
		return
	}
	// no deadline
	if deadline.IsZero() {
		return
	}
	// add timeout
	link := res.AcquireOperation1(OneshotOperation, time.Time{})
	link.PrepareLinkTimeout(deadline)
	op.link = link
	op.flags |= op_f_timeout
	op.sqeFlags |= liburing.IOSQE_IO_LINK
	op.promise.Future().LinkTimeout(link.promise.Future())
	return
}

func (res *Resource) ReleaseOperation1(op *Operation) {
	if op == nil {
		return
	}
	if op.releaseAble() {
		if link := op.link; link != nil {
			res.ReleaseOperation1(link)
		}
		switch future := op.promise.(type) {
		case *OneshotFuture:
			future.baseFuture.timeout = nil
			res.oneshotFutures.Put(future)
			break
		case *ZeroCopyFuture:
			future.baseFuture.timeout = nil
			future.result.N = 0
			future.result.Flags = 0
			future.result.Err = nil
			future.result.Attachment = nil
			res.zeroCopyFutures.Put(future)
			break
		case *MultishotFuture:
			future.baseFuture.timeout = nil
			future.adaptor = nil
			res.multishotFutures.Put(future)
			break
		default:
			panic(errors.New("invalid operation promise kind"))
			return
		}
		op.reset()
		res.operations.Put(op)
	}
}

func (res *Resource) AcquireOperation() *Operation {
	v := res.operations.Get()
	if v == nil {
		return &Operation{
			code:     liburing.IORING_OP_LAST,
			flags:    op_f_borrowed,
			resultCh: make(chan Result, 2),
		}
	}
	op := v.(*Operation)
	return op
}

func (res *Resource) ReleaseOperation(op *Operation) {
	if op == nil {
		return
	}
	if op.releaseAble() {
		if link := op.link; link != nil {
			link.reset()
			res.operations.Put(link)
		}
		op.reset()
		res.operations.Put(op)
	}
}

func (res *Resource) AcquireTimer(timeout time.Duration) *time.Timer {
	v := res.timers.Get()
	if v == nil {
		timer := time.NewTimer(timeout)
		return timer
	}
	timer := v.(*time.Timer)
	timer.Reset(timeout)
	return timer
}

func (res *Resource) ReleaseTimer(timer *time.Timer) {
	timer.Stop()
	res.timers.Put(timer)
}

func (res *Resource) AcquireMsg(b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32) *syscall.Msghdr {
	var msg *syscall.Msghdr
	msg0 := res.msgs.Get()
	if msg0 == nil {
		msg = &syscall.Msghdr{}
	} else {
		msg = msg0.(*syscall.Msghdr)
	}
	bLen := len(b)
	if bLen > 0 {
		msg.Iov = &syscall.Iovec{
			Base: &b[0],
			Len:  uint64(bLen),
		}
		msg.Iovlen = 1
	}
	oobLen := len(oob)
	if oobLen > 0 {
		msg.Control = &oob[0]
		msg.SetControllen(oobLen)
	}
	if addr != nil {
		msg.Name = (*byte)(unsafe.Pointer(addr))
		msg.Namelen = uint32(addrLen)
	}
	msg.Flags = flags
	return msg
}

func (res *Resource) ReleaseMsg(msg *syscall.Msghdr) {
	msg.Name = nil
	msg.Namelen = 0
	msg.Iov = nil
	msg.Iovlen = 0
	msg.Control = nil
	msg.Controllen = 0
	msg.Flags = 0
	res.msgs.Put(msg)
	return
}
