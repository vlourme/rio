//go:build linux

package aio

import (
	"runtime"
)

func Cancel(op *Operator) {
	cylinder := op.cylinder
	addr := uintptr(op.ptr())
	for i := 0; i < 10; i++ {
		if !op.Processing() {
			break
		}
		err := cylinder.prepareRW(opAsyncCancel, -1, addr, 0, 0, 0, 0)
		if err == nil {
			break
		}
		if IsBusy(err) {
			continue
		}
	}
	runtime.KeepAlive(op)
}
