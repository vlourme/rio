//go:build windows

package sockets

import (
	"golang.org/x/sys/windows"
	"runtime"
	"unsafe"
)

func newPoller(cnt int, cphandle windows.Handle) *poller {
	if cnt < 1 {
		cnt = runtime.NumCPU() * 2
	}
	return &poller{
		cphandle: cphandle,
		cnt:      cnt,
	}
}

type poller struct {
	cphandle windows.Handle
	key      uintptr
	cnt      int
}

func (pl *poller) start() {
	for i := 0; i < pl.cnt; i++ {
		go pl.wait()
	}
}

func (pl *poller) wait() {
	var key uintptr = 0
	for {
		var qty uint32
		var overlapped *windows.Overlapped
		getQueuedCompletionStatusErr := windows.GetQueuedCompletionStatus(pl.cphandle, &qty, &key, &overlapped, windows.INFINITE)
		if qty == 0 && overlapped == nil { // exit
			break
		}
		op := (*operation)(unsafe.Pointer(overlapped))
		op.complete(int(qty), getQueuedCompletionStatusErr)
	}
}

func (pl *poller) stop() {
	for i := 0; i < pl.cnt; i++ {
		_ = windows.PostQueuedCompletionStatus(pl.cphandle, 0, 0, nil)
	}
}
