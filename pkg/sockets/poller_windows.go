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
			if op.mode == disconnect && errors.Is(getQueuedCompletionStatusErr, windows.ERROR_CONNECTION_ABORTED) {
				// closed by server
				op.handleDisconnect()
				continue
			}
			op.failed(getQueuedCompletionStatusErr)
			continue
		}
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
		case disconnect:
			op.handleDisconnect()
			break
			// todo
		default:
			// not supported
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
