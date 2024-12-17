//go:build windows

package aio

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"runtime"
	"unsafe"
)

const dwordMax = 0xffffffff

func createSubIoCompletionPort(handle windows.Handle) error {
	eng := engine()
	cfd := windows.Handle(eng.fd)
	_, createErr := windows.CreateIoCompletionPort(handle, cfd, 0, 0)
	if createErr != nil {
		return os.NewSyscallError("iocp_create_io_completion_port", createErr)
	}
	return nil
}

func (engine *Engine) Start() {
	engine.lock.Lock()
	defer engine.lock.Unlock()
	if engine.running {
		panic(errors.New("aio: engine start failed cause already running"))
		return
	}

	var data windows.WSAData
	startupErr := windows.WSAStartup(uint32(0x202), &data)
	if startupErr != nil {
		panic(fmt.Errorf("aio: engine start failed, %v", startupErr))
		return
	}
	// settings
	settings := ResolveSettings[IOCPSettings](engine.settings)
	// threadCount
	threadCount := settings.ThreadCNT
	if threadCount == 0 {
		threadCount = dwordMax
	}
	// iocp
	cphandle, createIOCPErr := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, threadCount)
	if createIOCPErr != nil {
		panic(fmt.Errorf("aio: engine start failed, %v", createIOCPErr))
		return
	}
	engine.fd = int(cphandle)
	// cylinders
	for i := 0; i < len(engine.cylinders); i++ {
		cylinder := newIOCPCylinder(cphandle)
		engine.cylinders[i] = cylinder
		engine.wg.Add(1)
		go func(engine *Engine, cylinder Cylinder) {
			defer engine.wg.Done()
			if engine.cylindersLockOSThread {
				runtime.LockOSThread()
			}
			cylinder.Loop()
			if engine.cylindersLockOSThread {
				runtime.UnlockOSThread()
			}
		}(engine, cylinder)
	}

	engine.running = true
}

func (engine *Engine) Stop() {
	engine.lock.Lock()
	defer engine.lock.Unlock()
	if !engine.running {
		return
	}

	runtime.SetFinalizer(engine, nil)

	fd := windows.Handle(engine.fd)
	if fd == windows.InvalidHandle {
		return
	}
	for _, cylinder := range engine.cylinders {
		cylinder.Stop()
	}
	engine.wg.Wait()
	_ = windows.CloseHandle(fd)
	engine.fd = 0

	engine.running = false
}

type IOCPSettings struct {
	ThreadCNT uint32
}

func newIOCPCylinder(cphandle windows.Handle) (cylinder Cylinder) {
	cylinder = &IOCPCylinder{
		fd: cphandle,
	}
	return
}

// IOCPCylinder
// 不支持 ACTIVES，无法控制在哪个里完成。
type IOCPCylinder struct {
	fd windows.Handle
}

func (cylinder *IOCPCylinder) Fd() int {
	return int(cylinder.fd)
}

func (cylinder *IOCPCylinder) Loop() {
	fd := cylinder.fd
	key := uintptr(0)
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
	runtime.KeepAlive(cylinder)
}

func (cylinder *IOCPCylinder) Stop() {
	fd := cylinder.fd
	if fd == windows.InvalidHandle {
		return
	}
	_ = windows.PostQueuedCompletionStatus(fd, 0, 0, nil)
	runtime.KeepAlive(cylinder)
}

func (cylinder *IOCPCylinder) Actives() int64 {
	return 0
}
