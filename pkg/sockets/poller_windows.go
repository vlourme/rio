//go:build windows

package sockets

import (
	"errors"
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
		//wg:       new(sync.WaitGroup),
	}
}

type poller struct {
	cphandle windows.Handle
	cnt      int
	//wg       *sync.WaitGroup
}

func (pl *poller) start() {
	for i := 0; i < pl.cnt; i++ {
		//pl.wg.Add(1)
		go pl.wait()
	}
}

func (pl *poller) wait() {
	var qty uint32
	var key uintptr
	var overlapped *windows.Overlapped
	for {
		getQueuedCompletionStatusErr := windows.GetQueuedCompletionStatus(pl.cphandle, &qty, &key, &overlapped, windows.INFINITE)
		if qty == 0 && overlapped == nil { // exit
			break
		}
		op := (*operation)(unsafe.Pointer(overlapped))
		if getQueuedCompletionStatusErr != nil {
			op.failed(getQueuedCompletionStatusErr)
			continue
		}
		// todo use op.finish() to handle callback ?
		switch op.mode {
		case accept:
			op.handleAccept()
			break
		case read:
			op.handleRead()
			break
		case write:
			op.handleWrite()
			break
			// todo
		default:
			// not supported
			// todo means no handler, so failed will do nothing
			op.failed(wrapSyscallError("GetQueuedCompletionStatus", errors.New("invalid operation")))
			break
		}
		// reset overlapped
		overlapped = nil
	}
	//pl.wg.Done()
}

func (pl *poller) stop() {
	for i := 0; i < pl.cnt; i++ {
		_ = windows.PostQueuedCompletionStatus(pl.cphandle, 0, 0, nil)
	}
	//pl.wg.Wait()
}
