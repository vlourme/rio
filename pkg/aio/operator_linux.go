//go:build linux

package aio

import (
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

type SendfileResult struct {
	file   int
	remain uint32
	pipe   []int
}

func newOperator(fd Fd) *Operator {
	return &Operator{
		cylinder:   nil,
		fd:         fd,
		handle:     -1,
		n:          0,
		oobn:       0,
		rsa:        nil,
		msg:        nil,
		sfr:        nil,
		callback:   nil,
		completion: nil,
		timeout:    0,
		timer:      nil,
	}
}

type Operator struct {
	cylinder   *IOURingCylinder
	fd         Fd
	handle     int
	n          uint32
	oobn       uint32
	rsa        *syscall.RawSockaddrAny
	msg        *syscall.Msghdr
	sfr        *SendfileResult
	callback   OperationCallback
	completion OperatorCompletion
	timeout    time.Duration
	timer      *operatorTimer
}

func (op *Operator) setCylinder(cylinder *IOURingCylinder) {
	op.cylinder = cylinder
}

func (op *Operator) tryPrepareTimeout(cylinder *IOURingCylinder) {
	if op.timeout > 0 {
		op.timer = getOperatorTimer()
		op.timer.Start(op.timeout, &operatorCanceler{
			op:       op,
			cylinder: cylinder,
		})
	}
}

func (op *Operator) deadlineExceeded() (ok bool) {
	if timer := op.timer; timer != nil {
		ok = timer.DeadlineExceeded()
	}
	return
}

func (op *Operator) tryResetTimeout() {
	if timer := op.timer; timer != nil {
		timer.Done()
		putOperatorTimer(timer)
		op.timer = nil
	}
}

func (op *Operator) reset() {
	op.cylinder = nil
	op.handle = -1
	op.n = 0
	op.oobn = 0
	op.rsa = nil
	op.msg = nil
	op.sfr = nil
	op.callback = nil
	op.completion = nil
	op.tryResetTimeout()
}

func (op *Operator) ptr() uint64 {
	return uint64(uintptr(unsafe.Pointer(op)))
}

type operatorCanceler struct {
	cylinder *IOURingCylinder
	op       *Operator
}

func (canceler *operatorCanceler) Cancel() {
	cylinder := canceler.cylinder
	op := canceler.op
	userdata := uintptr(unsafe.Pointer(op))
	for i := 0; i < 10; i++ {
		err := cylinder.prepareRW(opAsyncCancel, -1, userdata, 0, 0, 0, 0)
		if err == nil {
			break
		}
		if IsBusy(err) {
			continue
		}
	}
	runtime.KeepAlive(op)
}
