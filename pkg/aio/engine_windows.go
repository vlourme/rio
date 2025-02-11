//go:build windows

package aio

import (
	"github.com/brickingsoft/errors"
	"golang.org/x/sys/windows"
	"os"
	"runtime"
	"unsafe"
)

const dwordMax = 0xffffffff

func createSubIoCompletionPort(handle windows.Handle) error {
	eng := engine()
	cfd := windows.Handle(eng.fd)
	if _, err := windows.CreateIoCompletionPort(handle, cfd, 0, 0); err != nil {
		err = errors.New(
			"create iocp failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(os.NewSyscallError("iocp_create_io_completion_port", err)),
		)
		return err
	}
	return nil
}

func (engine *Engine) Start() {
	engine.lock.Lock()
	defer engine.lock.Unlock()
	if engine.running {
		engine.startupErr = errors.New(
			"engine start failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(errors.Define("engine is already running")),
		)
		return
	}

	var data windows.WSAData
	startupErr := windows.WSAStartup(uint32(0x202), &data)
	if startupErr != nil {
		engine.startupErr = errors.New(
			"engine start failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(startupErr),
		)
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
		engine.startupErr = errors.New(
			"engine start failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(createIOCPErr),
		)
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

func nextIOCPCylinder() *IOCPCylinder {
	return nextCylinder().(*IOCPCylinder)
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
		op.received.Store(true)
		// handle iocp errors
		if getQueuedCompletionStatusErr != nil {
			// handle timeout
			getQueuedCompletionStatusErr = errors.From(ErrUnexpectedCompletion, errors.WithWrap(getQueuedCompletionStatusErr))
		}
		// complete
		if completion := op.completion; completion != nil {
			completion(int(qty), op, getQueuedCompletionStatusErr)
		}
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
