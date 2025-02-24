package aio

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"syscall"
)

const (
	maxSendfileSize = 4 << 20
	maxMMapSize     = int(^uint(0) >> 1)
)

var (
	pagesize = os.Getpagesize()
)

func (vortex *Vortex) Sendfile(ctx context.Context, dst int, r io.Reader, useZC bool) (written int64, err error) {
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
		return vortex.sendfileChunk(ctx, dst, srcFd, remain, useZC)
	}
	// mmap
	b, mmapErr := unix.Mmap(srcFd, 0, int(remain), unix.PROT_READ, unix.MAP_SHARED)
	if mmapErr != nil {
		err = os.NewSyscallError("mmap", mmapErr)
		return
	}
	defer func(b []byte) {
		_ = unix.Munmap(b)
	}(b)
	// madvise
	if advErr := unix.Madvise(b, unix.MADV_SEQUENTIAL); advErr != nil {
		err = os.NewSyscallError("madvise", mmapErr)
		return
	}
	defer func(b []byte) {
		_ = unix.Madvise(b, unix.MADV_FREE)
	}(b)

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
		if useZC {
			future := vortex.PrepareSendZC(ctx, dst, b[written:written+int64(chunk)], 0)
			n, wErr = future.Await(ctx)
		} else {
			future := vortex.PrepareSend(ctx, dst, b[written:written+int64(chunk)], 0)
			n, wErr = future.Await(ctx)
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

func (vortex *Vortex) sendfileChunk(ctx context.Context, dst int, src int, remain int64, useZC bool) (written int64, err error) {
	adv := false
	chunk := int64(pagesize)
	for err == nil && remain > 0 {
		if chunk > remain {
			chunk = remain
		}

		b, mmapErr := unix.Mmap(src, written, int(chunk), unix.PROT_READ, unix.MAP_SHARED)
		if mmapErr != nil {
			err = os.NewSyscallError("mmap", mmapErr)
			break
		}
		if advErr := unix.Madvise(b, unix.MADV_SEQUENTIAL); advErr == nil {
			adv = true
		}

		var (
			n    int
			wErr error
		)
		if useZC {
			future := vortex.PrepareSendZC(ctx, dst, b, 0)
			n, wErr = future.Await(ctx)
		} else {
			future := vortex.PrepareSend(ctx, dst, b, 0)
			n, wErr = future.Await(ctx)
		}
		if n > 0 {
			written += int64(n)
			remain -= int64(n)
		}
		err = wErr

		if adv {
			_ = unix.Madvise(b, unix.MADV_FREE)
		}
		if munmapErr := unix.Munmap(b); munmapErr != nil {
			err = os.NewSyscallError("munmap", munmapErr)
			break
		}
	}
	return
}
