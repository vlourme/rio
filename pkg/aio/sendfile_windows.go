//go:build windows

package aio

import (
	"github.com/brickingsoft/errors"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"unsafe"
)

func Sendfile(fd NetFd, filepath string, cb OperationCallback) {
	if len(filepath) == 0 {
		err := errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(errors.Define("aio.Sendfile: filepath is empty")),
		)
		cb(Userdata{}, err)
		return
	}
	src, openErr := windows.Open(filepath, windows.O_RDONLY|windows.O_NONBLOCK|windows.FILE_FLAG_SEQUENTIAL_SCAN, 0777)
	if openErr != nil {
		err := errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(os.NewSyscallError("open", openErr)),
		)
		cb(Userdata{}, err)
		return
	}

	curpos, seekToCurrentErr := windows.Seek(src, 0, io.SeekCurrent)
	if seekToCurrentErr != nil {
		_ = windows.Close(src)
		err := errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(os.NewSyscallError("seek", seekToCurrentErr)),
		)
		cb(Userdata{}, err)
		return
	}
	// find the number of b offset from curpos until the end of the file.
	remain, seekToEndErr := windows.Seek(src, -curpos, io.SeekEnd)
	if seekToEndErr != nil {
		_ = windows.Close(src)
		err := errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(os.NewSyscallError("seek", seekToEndErr)),
		)
		cb(Userdata{}, err)
		return
	}
	// now seek back to the original position.
	if _, seekToStart := windows.Seek(src, curpos, io.SeekStart); seekToStart != nil {
		_ = windows.Close(src)
		err := errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(os.NewSyscallError("seek", seekToStart)),
		)
		cb(Userdata{}, err)
		return
	}

	sendfile(fd, src, curpos, remain, 0, cb)
}

const maxChunkSizePerCall = int64(0x7fffffff - 1)

func sendfile(fd NetFd, file windows.Handle, curpos int64, remain int64, written int, cb OperationCallback) {
	op := acquireOperator(fd)

	op.completion = completeSendfile
	op.callback = cb

	chunkSize := maxChunkSizePerCall
	if chunkSize > remain {
		chunkSize = remain
	}
	// sfr
	op.sfr.file = file
	op.sfr.curpos = curpos
	op.sfr.remain = remain
	op.sfr.written = written

	op.n = uint32(chunkSize)
	op.overlapped.Offset = uint32(curpos)
	op.overlapped.OffsetHigh = uint32(curpos >> 32)

	wsaOverlapped := (*windows.Overlapped)(unsafe.Pointer(op))

	dst := windows.Handle(fd.Fd())

	err := windows.TransmitFile(dst, file, op.n, 0, wsaOverlapped, nil, windows.TF_WRITE_BEHIND)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		_ = windows.Close(file)
		err = errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(os.NewSyscallError("transmit_file", err)),
		)
		cb(Userdata{}, err)
		releaseOperator(op)
		return
	}
	return
}

func completeSendfile(result int, op *Operator, err error) {
	cb := op.callback
	sfr := op.sfr
	fd := op.fd
	releaseOperator(op)

	src := sfr.file

	if err != nil {
		_ = windows.Close(src)
		err = errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(os.NewSyscallError("transmit_file", err)),
		)
		cb(Userdata{}, err)
		return
	}

	curpos := sfr.curpos
	remain := sfr.remain
	written := sfr.written + result

	curpos += int64(result)

	if _, seekToStart := windows.Seek(src, curpos, io.SeekStart); seekToStart != nil {
		_ = windows.Close(src)
		err = errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(os.NewSyscallError("seek", seekToStart)),
		)
		cb(Userdata{N: written}, err)
		return
	}

	remain -= int64(result)
	if remain > 0 {
		dstFd := fd.(*netFd)
		sendfile(dstFd, src, curpos, remain, written, cb)
		return
	}

	_ = windows.Close(src)
	cb(Userdata{N: written}, nil)
	return
}
