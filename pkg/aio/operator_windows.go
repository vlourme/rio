//go:build windows

package aio

import (
	"errors"
	"syscall"
	"time"
)

//func NewOperator(fd Fd) *Operator {
//	return &Operator{
//		overlapped: windows.Overlapped{},
//		fd:         fd,
//		userdata:   Userdata{},
//		callback:   nil,
//		completion: nil,
//		timer:      nil,
//	}
//}

type Operator struct {
	overlapped syscall.Overlapped
	fd         Fd
	userdata   Userdata
	callback   OperationCallback
	completion OperatorCompletion
	timeout    time.Duration
	timer      *operatorTimer
}

func (op Operator) Close(cb OperationCallback) {
	switch op.fd.(type) {
	case NetFd:
		handle := op.fd.Fd()
		err := syscall.Closesocket(syscall.Handle(handle))
		if err != nil {
			err = errors.Join(errors.New("aio.Operator: close failed"), err)
		}
		cb(handle, op.userdata, err)
		return
	case FileFd:
		cb(0, op.userdata, errors.New("aio.Operator: close was not supported"))
		return
	default:
		cb(0, op.userdata, errors.New("aio.Operator: close was not supported"))
		return
	}
}

func (op Operator) Cancel() {
	handle := op.fd.Fd()
	_ = syscall.CancelIoEx(syscall.Handle(handle), &op.overlapped)
}
