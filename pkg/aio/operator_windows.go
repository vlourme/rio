//go:build windows

package aio

import (
	"golang.org/x/sys/windows"
	"syscall"
	"time"
)

type SendfileResult struct {
	file    windows.Handle
	curpos  int64
	remain  int64
	written int
}

func newOperator(fd Fd) *Operator {
	return &Operator{
		overlapped: syscall.Overlapped{},
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
	overlapped syscall.Overlapped
	fd         Fd
	handle     int
	n          uint32
	oobn       uint32
	rsa        *syscall.RawSockaddrAny
	msg        *windows.WSAMsg
	sfr        *SendfileResult
	callback   OperationCallback
	completion OperatorCompletion
	timeout    time.Duration
	timer      *operatorTimer
}

func (op *Operator) tryPrepareTimeout() {
	if op.timeout > 0 {
		op.timer = getOperatorTimer()
		op.timer.Start(op.timeout, &operatorCanceler{
			op: op,
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
	op.overlapped = syscall.Overlapped{}
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

type operatorCanceler struct {
	op *Operator
}

func (canceler *operatorCanceler) Cancel() {
	if op := canceler.op; op != nil {
		if fd := op.fd; fd != nil {
			handle := syscall.Handle(fd.Fd())
			overlapped := &op.overlapped
			_ = syscall.CancelIoEx(handle, overlapped)

		}
	}
}
