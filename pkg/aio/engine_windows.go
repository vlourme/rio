//go:build windows

package aio

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"runtime"
	"sync"
	"unsafe"
)

const dwordMax = 0xffffffff

var (
	key         uintptr         = 0
	cylindersWG *sync.WaitGroup = &sync.WaitGroup{}
	threadCount uint32          = 0
)

func createSubIoCompletionPort(handle windows.Handle) (windows.Handle, error) {
	eng := engine()
	fd := windows.Handle(eng.fd)
	if fd == windows.InvalidHandle {
		return windows.InvalidHandle, errors.New("aio: root iocp handle was not init")
	}
	h, createErr := windows.CreateIoCompletionPort(handle, fd, key, 0)
	if createErr != nil {
		return windows.InvalidHandle, os.NewSyscallError("iocp.CreateIoCompletionPort", createErr)
	}
	return h, nil
}

func (engine *Engine) Start() {
	var data windows.WSAData
	startupErr := windows.WSAStartup(uint32(0x202), &data)
	if startupErr != nil {
		panic(fmt.Errorf("aio: engine start failed, %v", startupErr))
		return
	}

	// threadCount
	settings := ResolveSettings[IOCPSettings](engine.settings)
	threadCount = settings.ThreadCNT
	if threadCount == 0 {
		threadCount = dwordMax
	}
	cphandle, createIOCPErr := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, threadCount)
	if createIOCPErr != nil {
		panic(fmt.Errorf("aio: engine start failed, %v", createIOCPErr))
		return
	}
	engine.fd = int(cphandle)

	// pollers
	cylinders := engine.cylinders
	if cylinders < 1 {
		cylinders = runtime.NumCPU() * 2
	}

	for i := 0; i < cylinders; i++ {
		cylindersWG.Add(1)
		go func(engine *Engine) {
			fd := windows.Handle(engine.fd)
			for {
				var qty uint32
				var overlapped *windows.Overlapped
				getQueuedCompletionStatusErr := windows.GetQueuedCompletionStatus(fd, &qty, &key, &overlapped, windows.INFINITE)
				if qty == 0 && overlapped == nil { // exit
					break
				}
				// convert to op
				op := (*Operator)(unsafe.Pointer(overlapped))
				// handle iocp errors
				if getQueuedCompletionStatusErr != nil {
					// handle timeout
					if timer := op.timer; timer != nil {
						if timer.DeadlineExceeded() {
							getQueuedCompletionStatusErr = errors.Join(ErrOperationDeadlineExceeded, getQueuedCompletionStatusErr)
						} else {
							timer.Done()
						}
						putOperatorTimer(timer)
						op.timer = nil
					}
					getQueuedCompletionStatusErr = errors.Join(ErrUnexpectedCompletion, getQueuedCompletionStatusErr)
				} else {
					// succeed and try close timer
					if timer := op.timer; timer != nil {
						timer.Done()
						putOperatorTimer(timer)
						op.timer = nil
					}
				}

				// complete op
				if completion := op.completion; completion != nil {
					completion(int(qty), op, getQueuedCompletionStatusErr)
					op.completion = nil
				}
				op.callback = nil

				runtime.KeepAlive(op)
			}
			cylindersWG.Done()
			runtime.KeepAlive(engine)
		}(engine)
	}
}

func (engine *Engine) Stop() {
	runtime.SetFinalizer(engine, nil)

	fd := windows.Handle(engine.fd)
	if fd == windows.InvalidHandle {
		return
	}
	for i := 0; i < engine.cylinders; i++ {
		_ = windows.PostQueuedCompletionStatus(fd, 0, key, nil)
	}
	cylindersWG.Wait()
	engine.fd = 0
}

type IOCPSettings struct {
	ThreadCNT uint32
}
