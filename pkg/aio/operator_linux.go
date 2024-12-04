//go:build linux

package aio

import (
	"runtime"
	"time"
	"unsafe"
)

type Operator struct {
	fd         Fd
	userdata   Userdata
	callback   OperationCallback
	completion OperatorCompletion
	timeout    time.Duration
	timer      *operatorTimer
}

type operatorCanceler struct {
	cylinder *IOURingCylinder
	op       *Operator
}

func (canceler *operatorCanceler) Cancel() {
	cylinder := canceler.cylinder
	op := canceler.op
	userdata := uint64(uintptr(unsafe.Pointer(op)))
	for i := 0; i < 5; i++ {
		err := cylinder.prepare(opAsyncCancel, -1, uintptr(userdata), 0, 0, 0, userdata)
		if err == nil {
			break
		}
		if IsBusyError(err) {
			continue
		}
	}
	runtime.KeepAlive(userdata)
}
