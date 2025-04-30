//go:build linux

package sys

import (
	"errors"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

var (
	pipePool = sync.Pool{New: func() interface{} {
		p := NewPipe()
		if p == nil {
			return nil
		}
		runtime.SetFinalizer(p, func(p *Pipe) {
			_ = p.Close()
		})
		return p
	}}
)

func AcquirePipe() (*Pipe, error) {
	v := pipePool.Get()
	if v == nil {
		return nil, syscall.EINVAL
	}
	return v.(*Pipe), nil
}

func ReleasePipe(pipe *Pipe) {
	if pipe.data != 0 {
		runtime.SetFinalizer(pipe, nil)
		_ = pipe.Close()
		return
	}
	pipePool.Put(pipe)
}

const (
	// MaxSpliceSize is the maximum amount of data Splice asks
	// the kernel to move in a single call to splice(2).
	// We use 1MB as Splice writes data through a pipe, and 1MB is the default maximum pipe buffer size,
	// which is determined by /proc/sys/fs/pipe-max-size.
	MaxSpliceSize = 1 << 20
)

func NewPipe() *Pipe {
	var fds [2]int
	if err := syscall.Pipe2(fds[:], syscall.O_CLOEXEC|syscall.O_NONBLOCK); err != nil {
		return nil
	}
	_, _ = Fcntl(fds[0], syscall.F_SETPIPE_SZ, MaxSpliceSize)
	return &Pipe{pipeFields: pipeFields{rfd: fds[0], wfd: fds[1]}}
}

type pipeFields struct {
	rfd  int
	wfd  int
	data int
}

type Pipe struct {
	pipeFields

	// We want to use a finalizer, so ensure that the size is
	// large enough to not use the tiny allocator.
	_ [24 - unsafe.Sizeof(pipeFields{})%24]byte
}

func (pipe *Pipe) SetMaxSize(size int32) {
	if size > 0 {
		_, _ = fcntl(int32(pipe.rfd), syscall.F_SETPIPE_SZ, size)
	}
}

func (pipe *Pipe) ReaderFd() int {
	return pipe.rfd
}

func (pipe *Pipe) WriterFd() int {
	return pipe.wfd
}

func (pipe *Pipe) DrainN(n int) {
	pipe.data += n
}

func (pipe *Pipe) PumpN(n int) {
	pipe.data -= n
}

func (pipe *Pipe) Close() (err error) {
	err = syscall.Close(pipe.rfd)
	if werr := syscall.Close(pipe.wfd); werr != nil {
		if err == nil {
			err = werr
		} else {
			err = errors.Join(err, werr)
		}
	}
	return
}
