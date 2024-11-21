//go:build windows

package sockets

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"runtime"
	"sync"
	"unsafe"
)

const dwordMax = 0xffffffff

var (
	key         uintptr         = 0
	pollersWG   *sync.WaitGroup = &sync.WaitGroup{}
	pollersNum  uint32          = 0
	threadCount uint32          = 0
)

func createSubIoCompletionPort(handle windows.Handle) (windows.Handle, error) {
	cc := getCompletions()
	fd := windows.Handle(cc.Fd())
	if fd == windows.InvalidHandle {
		return windows.InvalidHandle, errors.New("sockets: root iocp handle was not init")
	}
	return windows.CreateIoCompletionPort(handle, fd, key, 0)
}

func (com *Completions) run() {
	var data windows.WSAData
	startupErr := windows.WSAStartup(uint32(0x202), &data)
	if startupErr != nil {
		panic(fmt.Sprintf("sockets: sockets Completions poll failed: %v", startupErr))
		return
	}

	options := com.Options()
	// threadCount
	threadCount = options.ThreadCPU
	if threadCount == 0 {
		threadCount = dwordMax
	}
	cphandle, createIOCPErr := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, threadCount)
	if createIOCPErr != nil {
		panic(fmt.Sprintf("sockets: sockets Completions poll failed: %v", createIOCPErr))
		return
	}
	com.fd = uintptr(cphandle)

	// pollers
	pollersNum = options.Pollers
	if pollersNum < 1 {
		pollersNum = uint32(runtime.NumCPU() * 2)
	}

	for i := uint32(0); i < pollersNum; i++ {
		pollersWG.Add(1)
		go func(com *Completions) {
			fd := windows.Handle(com.Fd())
			for {
				var qty uint32
				var overlapped *windows.Overlapped
				getQueuedCompletionStatusErr := windows.GetQueuedCompletionStatus(fd, &qty, &key, &overlapped, windows.INFINITE)
				if qty == 0 && overlapped == nil { // exit
					break
				}
				// convert to op
				op := (*operation)(unsafe.Pointer(overlapped))
				// handle iocp errors
				if getQueuedCompletionStatusErr != nil {
					// handle timeout
					if timer := op.timer; timer != nil {
						timer.Done()
						putOperationTimer(timer)
						op.timer = nil
						if op.deadlineExceeded {
							getQueuedCompletionStatusErr = errors.Join(ErrOperationDeadlineExceeded, getQueuedCompletionStatusErr)
							op.deadlineExceeded = false
						}
					}
					getQueuedCompletionStatusErr = errors.Join(ErrUnexpectedCompletion, getQueuedCompletionStatusErr)
				} else {
					if timer := op.timer; timer != nil {
						timer.Done()
						putOperationTimer(timer)
						op.timer = nil
					}
				}
				op.complete(int(qty), getQueuedCompletionStatusErr)
				runtime.KeepAlive(op.conn)
			}
			pollersWG.Done()
			runtime.KeepAlive(com)
		}(com)
	}
}

func (com *Completions) shutdown() {
	runtime.SetFinalizer(com, nil)

	fd := windows.Handle(com.Fd())
	if fd == windows.InvalidHandle {
		return
	}
	for i := uint32(0); i < pollersNum; i++ {
		_ = windows.PostQueuedCompletionStatus(fd, 0, key, nil)
	}
	pollersWG.Wait()
	com.fd = ^uintptr(0)
}
