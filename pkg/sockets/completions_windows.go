//go:build windows

package sockets

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"runtime"
	"sync"
	"unsafe"
)

var (
	iocp    windows.Handle  = windows.InvalidHandle
	cpus    int             = runtime.NumCPU() * 2
	key     uintptr         = 0
	threads *sync.WaitGroup = &sync.WaitGroup{}
)

func createSubIoCompletionPort(handle windows.Handle) (windows.Handle, error) {
	if iocp == windows.InvalidHandle {
		return windows.InvalidHandle, errors.New("root iocp handle was not init")
	}
	return windows.CreateIoCompletionPort(handle, iocp, key, 0)
}

func (com *completions) poll() {
	var data windows.WSAData
	startupErr := windows.WSAStartup(uint32(0x202), &data)
	if startupErr != nil {
		panic(fmt.Sprintf("sockets completions poll failed: %v", startupErr))
		return
	}
	cphandle, createIOCPErr := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 0)
	if createIOCPErr != nil {
		panic(fmt.Sprintf("sockets completions poll failed: %v", createIOCPErr))
		return
	}
	iocp = cphandle
	for i := 0; i < cpus; i++ {
		threads.Add(1)
		go func() {
			for {
				var qty uint32
				var overlapped *windows.Overlapped
				getQueuedCompletionStatusErr := windows.GetQueuedCompletionStatus(iocp, &qty, &key, &overlapped, windows.INFINITE)
				if qty == 0 && overlapped == nil { // exit
					break
				}
				if errors.Is(windows.ERROR_TIMEOUT, getQueuedCompletionStatusErr) {
					getQueuedCompletionStatusErr = context.DeadlineExceeded
				}
				op := (*operation)(unsafe.Pointer(overlapped))
				op.complete(int(qty), getQueuedCompletionStatusErr)
			}
			threads.Done()
		}()
	}
}

func (com *completions) shutdown() {
	if iocp == windows.InvalidHandle {
		return
	}
	for i := 0; i < cpus; i++ {
		_ = windows.PostQueuedCompletionStatus(iocp, 0, key, nil)
	}
	threads.Wait()
	iocp = windows.InvalidHandle
}
