//go:build linux

package aio

import (
	"context"
	"errors"
	"io"
	"os"
	"syscall"
	"time"
)

const (
	maxSendfileSize = 4 << 20
	maxMMapSize     = int(^uint(0) >> 1)
)

var (
	pagesize = os.Getpagesize()
)

func (vortex *Vortex) Sendfile(ctx context.Context, dst int, r io.Reader, useSendZC bool) (written int64, err error) {
	var remain int64 = 0
	lr, ok := r.(*io.LimitedReader)
	if ok {
		remain, r = lr.N, lr.R
		if remain <= 0 {
			return 0, nil
		}
	}

	file, isFile := r.(*os.File)
	if !isFile {
		return 0, errors.New("cannot send file to non-file destination")
	}
	if remain == 0 {
		info, infoErr := file.Stat()
		if infoErr != nil {
			err = infoErr
			return
		}
		remain = info.Size()
	}

	srcFd := int(file.Fd())

	if remain > int64(maxMMapSize) {
		return vortex.sendfileChunk(ctx, dst, srcFd, remain, useSendZC)
	}
	// mmap
	b, mmapErr := mmap(srcFd, 0, int(remain), syscall.PROT_READ, syscall.MAP_SHARED)
	if mmapErr != nil {
		err = os.NewSyscallError("mmap", mmapErr)
		return
	}
	defer func(b []byte) {
		_ = munmap(b)
	}(b)
	// madvise
	if advErr := madvise(b, syscall.MADV_WILLNEED|syscall.MADV_SEQUENTIAL); advErr != nil {
		err = os.NewSyscallError("madvise", mmapErr)
		return
	}

	chunk, chunkErr := syscall.GetsockoptInt(dst, syscall.SOL_SOCKET, syscall.SO_SNDBUF)
	if chunkErr != nil {
		chunk = maxSendfileSize
	}

	for err == nil && remain > 0 {
		if int64(chunk) > remain {
			chunk = int(remain)
		}
		var (
			n    int
			wErr error
		)
		if useSendZC {
			n, wErr = vortex.SendZC(ctx, dst, b[written:written+int64(chunk)], time.Time{})
		} else {
			n, wErr = vortex.Send(ctx, dst, b[written:written+int64(chunk)], time.Time{})
		}
		if n > 0 {
			written += int64(n)
			remain -= int64(n)
		}
		err = wErr
	}

	if lr != nil {
		lr.N -= written
	}

	return
}

func (vortex *Vortex) sendfileChunk(ctx context.Context, dst int, src int, remain int64, useSendZC bool) (written int64, err error) {
	chunk := int64(pagesize)
	for err == nil && remain > 0 {
		if chunk > remain {
			chunk = remain
		}
		b, mmapErr := mmap(src, written, int(chunk), syscall.PROT_READ, syscall.MAP_SHARED)
		if mmapErr != nil {
			err = os.NewSyscallError("mmap", mmapErr)
			break
		}
		_ = madvise(b, syscall.MADV_WILLNEED|syscall.MADV_SEQUENTIAL)

		var (
			n    int
			wErr error
		)
		if useSendZC {
			n, wErr = vortex.SendZC(ctx, dst, b, time.Time{})
		} else {
			n, wErr = vortex.Send(ctx, dst, b, time.Time{})
		}
		if n > 0 {
			written += int64(n)
			remain -= int64(n)
		}
		err = wErr

		if munmapErr := munmap(b); munmapErr != nil {
			err = os.NewSyscallError("munmap", munmapErr)
			break
		}
	}
	return
}
