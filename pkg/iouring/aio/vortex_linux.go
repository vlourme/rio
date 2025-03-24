//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

func Open(options ...Option) (v *Vortex, err error) {
	// check kernel version
	version := iouring.GetVersion()
	if version.Invalidate() {
		err = errors.New("get kernel version failed")
		return
	}
	if !version.GTE(6, 0, 0) {
		err = errors.New("kernel version must greater than or equal to 6.0")
		return
	}
	// options
	opt := Options{}
	for _, option := range options {
		option(&opt)
	}
	// ring
	ring, ringErr := OpenIOURing(opt)
	if ringErr != nil {
		err = ringErr
		return
	}
	// vortex
	v = &Vortex{
		IOURing: ring,
		operations: sync.Pool{
			New: func() interface{} {
				return &Operation{
					code:     iouring.OpLast,
					flags:    borrowed,
					resultCh: make(chan Result, 1),
				}
			},
		},
		msgs: sync.Pool{
			New: func() interface{} {
				return &syscall.Msghdr{}
			},
		},
	}
	return
}

type Vortex struct {
	IOURing
	operations sync.Pool
	msgs       sync.Pool
}

func (vortex *Vortex) FixedFdInstall(directFd int) (regularFd int, err error) {
	op := vortex.acquireOperation()
	op.PrepareFixedFdInstall(directFd)
	regularFd, _, err = vortex.submitAndWait(op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) CancelOperation(op *Operation) (n int, cqeFlags uint32, err error) {
	if err = vortex.tryCancelOperation(op); err != nil { // cancel succeed
		r, ok := <-op.resultCh
		if !ok {
			op.Close()
			err = ErrCanceled
			return
		}
		n, cqeFlags, err = r.N, r.Flags, r.Err
		if errors.Is(err, syscall.ECANCELED) {
			err = ErrTimeout
		}
		return
	}
	return
}

func (vortex *Vortex) tryCancelOperation(target *Operation) (err error) {
	if target.cancelAble() {
		op := vortex.acquireOperation()
		op.PrepareCancel(target)
		_, _, err = vortex.submitAndWait(op)
		vortex.releaseOperation(op)
		if err != nil && !IsOperationInvalid(err) { // discard no op to cancel
			err = nil
		}
	}
	return
}

func (vortex *Vortex) acquireOperation() *Operation {
	op := vortex.operations.Get().(*Operation)
	return op
}

func (vortex *Vortex) releaseOperation(op *Operation) {
	if op.releaseAble() {
		op.reset()
		vortex.operations.Put(op)
	}
}

func (vortex *Vortex) acquireMsg(b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32) *syscall.Msghdr {
	msg := vortex.msgs.Get().(*syscall.Msghdr)
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

func (vortex *Vortex) releaseMsg(msg *syscall.Msghdr) {
	msg.Name = nil
	msg.Namelen = 0
	msg.Iov = nil
	msg.Iovlen = 0
	msg.Control = nil
	msg.Controllen = 0
	msg.Flags = 0
	vortex.msgs.Put(msg)
	return
}

func (vortex *Vortex) submitAndWait(op *Operation) (n int, cqeFlags uint32, err error) {
	// attach timeout op
	if op.timeout != nil {
		timeoutOp := vortex.acquireOperation()
		defer vortex.releaseOperation(timeoutOp)
		timeoutOp.prepareLinkTimeout(op)
	}
RETRY:
	if ok := vortex.Submit(op); ok {
		n, cqeFlags, err = vortex.awaitOperation(op)
		if err != nil {
			if errors.Is(err, ErrIOURingSQBusy) { // means cannot get sqe
				goto RETRY
			}
			return
		}
		return
	}
	err = ErrCanceled
	return
}

func (vortex *Vortex) awaitOperation(op *Operation) (n int, cqeFlags uint32, err error) {
	r, ok := <-op.resultCh
	if !ok {
		op.Close()
		err = ErrCanceled
		return
	}
	n, cqeFlags, err = r.N, r.Flags, r.Err

	if op.timeout != nil && op.addr2 != nil { // wait timeout op
		timeoutOp := (*Operation)(op.addr2)
		r, ok = <-timeoutOp.resultCh
		if ok {
			if err != nil && errors.Is(r.Err, syscall.ETIME) {
				err = ErrTimeout
			}
		}
	}

	if err != nil {
		if errors.Is(err, syscall.ECANCELED) {
			err = ErrCanceled
		} else if errors.Is(r.Err, syscall.ETIME) {
			err = ErrTimeout
		} else {
			err = os.NewSyscallError(op.Name(), err)
		}
	}
	return
}
