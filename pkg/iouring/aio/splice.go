//go:build linux

package aio

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

const (
	MaxSpliceSize = 1 << 20
)

func (vortex *Vortex) Splice(ctx context.Context, dst int, src int, remain int64) (n int64, err error) {
	pipe, pipeErr := AcquireSplicePipe()
	if pipeErr != nil {
		return 0, pipeErr
	}
	defer ReleaseSplicePipe(pipe)

	for err == nil && remain > 0 {
		chunk := int64(MaxSpliceSize)
		if chunk > remain {
			chunk = remain
		}
		// drain
		drainFuture := vortex.PrepareSplice(src, -1, pipe.wfd, -1, uint32(chunk), unix.SPLICE_F_NONBLOCK)
		drained, drainedErr := drainFuture.Await(ctx)
		if drainedErr != nil || drained == 0 {
			err = drainedErr
			break
		}
		pipe.DrainN(drained)
		// pump
		pumpFuture := vortex.PrepareSplice(pipe.rfd, -1, dst, -1, uint32(drained), unix.SPLICE_F_NONBLOCK)
		pumped, pumpedErr := pumpFuture.Await(ctx)
		if pumped > 0 {
			n += int64(pumped)
			remain -= int64(pumped)
			pipe.PumpN(pumped)
		}
		err = pumpedErr
	}
	return
}

var (
	splicePipePool = sync.Pool{New: func() interface{} {
		p := NewSplicePipe()
		if p == nil {
			return nil
		}
		runtime.SetFinalizer(p, func(p *SplicePipe) {
			_ = p.Close()
		})
		return p
	}}
)

func AcquireSplicePipe() (*SplicePipe, error) {
	v := splicePipePool.Get()
	if v == nil {
		return nil, syscall.EINVAL
	}
	return v.(*SplicePipe), nil
}

func ReleaseSplicePipe(pipe *SplicePipe) {
	if pipe.data != 0 {
		runtime.SetFinalizer(pipe, nil)
		_ = pipe.Close()
		return
	}
	splicePipePool.Put(pipe)
}

func NewSplicePipe() *SplicePipe {
	var fds [2]int
	if err := syscall.Pipe2(fds[:], syscall.O_CLOEXEC|syscall.O_NONBLOCK); err != nil {
		return nil
	}

	// Splice will loop writing maxSpliceSize bytes from the source to the pipe,
	// and then write those bytes from the pipe to the destination.
	// Set the pipe buffer size to maxSpliceSize to optimize that.
	// Ignore errors here, as a smaller buffer size will work,
	// although it will require more system calls.
	_, _ = fcntl(fds[0], syscall.F_SETPIPE_SZ, MaxSpliceSize)

	return &SplicePipe{splicePipeFields: splicePipeFields{rfd: fds[0], wfd: fds[1]}}
}

type splicePipeFields struct {
	rfd  int
	wfd  int
	data int
}

type SplicePipe struct {
	splicePipeFields

	// We want to use a finalizer, so ensure that the size is
	// large enough to not use the tiny allocator.
	_ [24 - unsafe.Sizeof(splicePipeFields{})%24]byte
}

func (pipe *SplicePipe) ReaderFd() int {
	return pipe.rfd
}

func (pipe *SplicePipe) WriterFd() int {
	return pipe.wfd
}

func (pipe *SplicePipe) DrainN(n int) {
	pipe.data += n
}

func (pipe *SplicePipe) PumpN(n int) {
	pipe.data -= n
}

func (pipe *SplicePipe) Close() (err error) {
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
