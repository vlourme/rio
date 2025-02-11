//go:build linux

package aio

import (
	"unsafe"
)

func CancelRead(fd Fd) {
	if op := fd.ROP(); op != nil {
		if received := op.received.Load(); received {
			return
		}
		addr := uintptr(unsafe.Pointer(op))
		cylinder := fd.Cylinder().(*IOURingCylinder)
		for i := 0; i < 10; i++ {
			if err := cylinder.prepareRW(opAsyncCancel, 0, addr, 0, 0, 0, 0); err != nil {
				if IsBusy(err) {
					continue
				}
				break
			}
			break
		}
	}
}

func CancelWrite(fd Fd) {
	if op := fd.WOP(); op != nil {
		if received := op.received.Load(); received {
			return
		}
		addr := uintptr(unsafe.Pointer(op))
		cylinder := fd.Cylinder().(*IOURingCylinder)
		for i := 0; i < 10; i++ {
			if err := cylinder.prepareRW(opAsyncCancel, 0, addr, 0, 0, 0, 0); err != nil {
				if IsBusy(err) {
					continue
				}
				break
			}
			break
		}
	}
}
