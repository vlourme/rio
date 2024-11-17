package sockets

import (
	"runtime"
	"sync"
)

var (
	comOptions                 = CompletionOptions{}
	comCreateOnce              = sync.Once{}
	com           *completions = nil
)

func SetInitCompletionOptions(options CompletionOptions) {
	comOptions = options
}

type CompletionOptions struct {
	// Pollers
	// 完成事件拉取者数量，默认为 runtime.NumCPU() * 2。
	Pollers uint32
	// Flags
	// IOURING的Flags
	Flags uint32
	// ThreadCPU
	// IOURING与IOCP的线程数
	ThreadCPU uint32
	// IOURING的线程闲置时长
	ThreadIdle uint32
}

func getCompletions() *completions {
	comCreateOnce.Do(func() {
		com = &completions{
			fd:      ^uintptr(0),
			options: comOptions,
		}
		runtime.SetFinalizer(com, (*completions).shutdown)
		com.run()
	})
	return com
}

type completions struct {
	fd      uintptr
	options CompletionOptions
}

func (com *completions) Fd() uintptr {
	return com.fd
}

func (com *completions) Options() CompletionOptions {
	return com.options
}
