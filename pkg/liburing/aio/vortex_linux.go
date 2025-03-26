//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

func Open(options ...Option) (v *Vortex, err error) {
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
					code:     liburing.IORING_OP_LAST,
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

func (vortex *Vortex) CancelOperation(target *Operation) (err error) {
	if target.cancelAble() {
		op := vortex.acquireOperation()
		op.PrepareCancel(target)
		_, _, err = vortex.submitAndWait(op)
		vortex.releaseOperation(op)
		if err != nil && !IsOperationInvalid(err) { // discard target missing
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
	if op.timeout != nil { // prepare timeout op
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
			if errors.Is(err, syscall.ECANCELED) && op.timeout != nil && op.addr2 != nil {
				op.timeout = nil // clean timeout for CQE_F_MORE, such as sendzc
				timeoutOp := (*Operation)(op.addr2)
				if timeoutErr := vortex.awaitTimeoutOp(timeoutOp); timeoutErr != nil {
					if errors.Is(timeoutErr, ErrTimeout) {
						err = ErrTimeout
					}
				}
			}
			return
		}
		if op.timeout != nil && op.addr2 != nil { // await timeout
			op.timeout = nil // clean timeout for CQE_F_MORE, such as sendzc
			timeoutOp := (*Operation)(op.addr2)
			_ = vortex.awaitTimeoutOp(timeoutOp)
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

	if err != nil {
		if errors.Is(err, syscall.ECANCELED) {
			err = ErrCanceled
		} else {
			err = os.NewSyscallError(op.Name(), err)
		}
	}
	return
}

func (vortex *Vortex) awaitTimeoutOp(timeoutOp *Operation) (err error) {
	r, ok := <-timeoutOp.resultCh
	if ok {
		if r.Err != nil && errors.Is(r.Err, syscall.ETIME) {
			err = ErrTimeout
		}
	}
	return
}
