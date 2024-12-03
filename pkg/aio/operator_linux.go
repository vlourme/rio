//go:build linux

package aio

import "time"

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
	userdata uint64
}

func (op *operatorCanceler) Cancel() {
	ring := op.cylinder.ring
	for {
		entry := ring.GetSQE()
		if entry == nil {
			if op.cylinder.stopped.Load() {
				return
			}
			continue
		}
		entry.prepareRW(opAsyncCancel, -1, 0, 0, 0, op.userdata, 0)
		break
	}
}
