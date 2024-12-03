//go:build linux

package aio

import (
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
	ring := cylinder.ring
	op := canceler.op
	opPtr := uint64(uintptr(unsafe.Pointer(op)))
	for {
		entry := ring.GetSQE()
		if entry == nil {
			if cylinder.stopped.Load() {
				return
			}
			continue
		}
		entry.prepareRW(opAsyncCancel, -1, 0, 0, 0, opPtr, 0)
		break
	}
}
