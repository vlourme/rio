package sockets

import (
	"errors"
	"runtime"
	"sync"
)

var (
	ErrCompleteFailed = errors.New("sockets: complete failed")
)

func IsCompleteFailed(err error) bool {
	return errors.Is(err, ErrCompleteFailed)
}

var (
	comOnce              = sync.Once{}
	com     *Completions = nil
)

func Startup(options CompletionOptions) {
	com = &Completions{
		fd:      ^uintptr(0),
		options: options,
	}
	com.run()
}

func Shutdown() {
	getCompletions().shutdown()
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

func getCompletions() *Completions {
	comOnce.Do(func() {
		if com == nil {
			com = &Completions{
				fd:      ^uintptr(0),
				options: CompletionOptions{},
			}
			runtime.SetFinalizer(com, (*Completions).shutdown)
			com.run()
		}
	})
	return com
}

type Completions struct {
	fd      uintptr
	options CompletionOptions
}

func (com *Completions) Fd() uintptr {
	return com.fd
}

func (com *Completions) Options() CompletionOptions {
	return com.options
}
