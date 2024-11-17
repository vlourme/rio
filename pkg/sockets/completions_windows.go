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

const dwordMax = 0xffffffff

var (
	key       uintptr         = 0
	threads   *sync.WaitGroup = &sync.WaitGroup{}
	pollers   uint32          = 0
	threadcnt uint32          = 0
)

func createSubIoCompletionPort(handle windows.Handle) (windows.Handle, error) {
	cc := getCompletions()
	fd := windows.Handle(cc.Fd())
	if fd == windows.InvalidHandle {
		return windows.InvalidHandle, errors.New("sockets: root iocp handle was not init")
	}
	return windows.CreateIoCompletionPort(handle, fd, key, 0)
}

func (com *completions) run() {
	var data windows.WSAData
	startupErr := windows.WSAStartup(uint32(0x202), &data)
	if startupErr != nil {
		panic(fmt.Sprintf("sockets: sockets completions poll failed: %v", startupErr))
		return
	}

	options := com.Options()
	// threadcnt
	threadcnt = options.ThreadCPU
	if threadcnt == 0 {
		threadcnt = dwordMax
	}
	cphandle, createIOCPErr := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, threadcnt)
	if createIOCPErr != nil {
		panic(fmt.Sprintf("sockets: sockets completions poll failed: %v", createIOCPErr))
		return
	}
	com.fd = uintptr(cphandle)

	// pollers
	pollers = options.Pollers
	if pollers < 1 {
		pollers = uint32(runtime.NumCPU() * 2)
	}

	for i := uint32(0); i < pollers; i++ {
		threads.Add(1)
		go func(com *completions) {
			fd := windows.Handle(com.Fd())
			for {
				var qty uint32
				var overlapped *windows.Overlapped
				getQueuedCompletionStatusErr := windows.GetQueuedCompletionStatus(fd, &qty, &key, &overlapped, windows.INFINITE)
				if qty == 0 && overlapped == nil { // exit
					break
				}
				if errors.Is(windows.ERROR_TIMEOUT, getQueuedCompletionStatusErr) {
					getQueuedCompletionStatusErr = context.DeadlineExceeded
				}
				op := (*operation)(unsafe.Pointer(overlapped))
				op.complete(int(qty), getQueuedCompletionStatusErr)
				runtime.KeepAlive(op.conn)
			}
			threads.Done()
		}(com)
	}
	runtime.KeepAlive(com)
}

func (com *completions) shutdown() {
	fd := windows.Handle(com.Fd())
	if fd == windows.InvalidHandle {
		return
	}
	for i := uint32(0); i < pollers; i++ {
		_ = windows.PostQueuedCompletionStatus(fd, 0, key, nil)
	}
	threads.Wait()
	com.fd = ^uintptr(0)
}
