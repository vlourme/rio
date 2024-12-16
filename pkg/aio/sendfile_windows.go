//go:build windows

package aio

import (
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"runtime"
	"unsafe"
)

func Sendfile(fd NetFd, filepath string, cb OperationCallback) {
	// op
	op := fd.WriteOperator()
	if len(filepath) == 0 {
		cb(0, op.userdata, errors.New("aio.Sendfile: filepath is empty"))
		return
	}
	src, openErr := windows.Open(filepath, windows.O_RDONLY|windows.O_NONBLOCK|windows.FILE_FLAG_SEQUENTIAL_SCAN, 0777)
	if openErr != nil {
		cb(0, op.userdata, os.NewSyscallError("open", openErr))
		return
	}

	curpos, seekToCurrentErr := windows.Seek(src, 0, io.SeekCurrent)
	if seekToCurrentErr != nil {
		_ = windows.Close(src)
		cb(0, op.userdata, os.NewSyscallError("seek", seekToCurrentErr))
		return
	}
	// find the number of bytes offset from curpos until the end of the file.
	remain, seekToEndErr := windows.Seek(src, -curpos, io.SeekEnd)
	if seekToEndErr != nil {
		_ = windows.Close(src)
		cb(0, op.userdata, os.NewSyscallError("seek", seekToEndErr))
		return
	}
	// now seek back to the original position.
	if _, seekToStart := windows.Seek(src, curpos, io.SeekStart); seekToStart != nil {
		_ = windows.Close(src)
		cb(0, op.userdata, os.NewSyscallError("seek", seekToStart))
		return
	}

	ffd := &fileFd{
		handle: int(src),
		path:   filepath,
		rop:    Operator{},
		wop:    Operator{},
	}
	ffd.rop.fd = ffd
	ffd.wop.fd = ffd

	sendfile(fd, ffd, curpos, remain, 0, cb)
}

const maxChunkSizePerCall = int64(0x7fffffff - 1)

func sendfile(fd NetFd, file FileFd, curpos int64, remain int64, wrote int, cb OperationCallback) {
	op := fd.WriteOperator()
	op.userdata.Fd = file
	op.completion = completeSendfile
	op.callback = cb

	chunkSize := maxChunkSizePerCall
	if chunkSize > remain {
		chunkSize = remain
	}

	op.userdata.Msg.BufferCount = uint32(remain)
	op.userdata.Msg.Control.Len = uint32(curpos)
	op.userdata.Msg.WSAMsg.Flags = uint32(wrote)

	op.userdata.QTY = uint32(chunkSize)
	op.overlapped.Offset = uint32(curpos)
	op.overlapped.OffsetHigh = uint32(curpos >> 32)

	wsaOverlapped := (*windows.Overlapped)(unsafe.Pointer(&op))

	dst := windows.Handle(fd.Fd())
	src := windows.Handle(file.Fd())

	// syscall.TransmitFile(o.fd.Sysfd, o.handle, o.qty, 0, &o.o, nil, syscall.TF_WRITE_BEHIND)
	err := windows.TransmitFile(dst, src, op.userdata.QTY, 0, wsaOverlapped, nil, windows.TF_WRITE_BEHIND)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		_ = windows.Close(src)
		// handle err
		cb(0, op.userdata, os.NewSyscallError("transmit_file", err))
		// reset
		op.callback = nil
		op.completion = nil
	}
	runtime.KeepAlive(op)
}

func completeSendfile(result int, op *Operator, err error) {
	src := windows.Handle(op.userdata.Fd.Fd())

	if err != nil {
		_ = windows.Close(src)
		err = os.NewSyscallError("transmit_file", err)
		op.callback(0, op.userdata, err)
		return
	}

	curpos := int64(op.userdata.Msg.Control.Len)
	remain := int64(op.userdata.Msg.BufferCount)
	written := int(op.userdata.Msg.WSAMsg.Flags) + result

	curpos += int64(result)

	if _, seekToStart := windows.Seek(src, curpos, io.SeekStart); seekToStart != nil {
		_ = windows.Close(src)
		op.callback(written, op.userdata, os.NewSyscallError("seek", seekToStart))
		return
	}

	remain -= int64(result)
	if remain > 0 {
		dstFd := op.fd.(NetFd)
		srcFd := op.userdata.Fd.(FileFd)
		sendfile(dstFd, srcFd, curpos, remain, written, op.callback)
		return
	}

	_ = windows.Close(src)
	op.callback(written, op.userdata, nil)
	return
}
