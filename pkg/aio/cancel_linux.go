//go:build linux

package aio

import "unsafe"

func Cancel(op *Operator) {
	cylinder := op.cylinder
	userdata := uint64(uintptr(unsafe.Pointer(op)))
	for i := 0; i < 10; i++ {
		err := cylinder.prepareRW(opAsyncCancel, -1, uintptr(userdata), 0, 0, 0, 0)
		if err == nil {
			break
		}
		if IsBusyError(err) {
			continue
		}
	}
}
