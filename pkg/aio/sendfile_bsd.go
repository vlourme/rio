//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"errors"
	"io"
	"os"
	"runtime"
	"syscall"
)

func Sendfile(fd NetFd, filepath string, cb OperationCallback) {
	// op
	op := WriteOperator(fd)
	if len(filepath) == 0 {
		cb(-1, Userdata{}, errors.New("aio.Sendfile: filepath is empty"))
		return
	}
	op.callback = cb
	op.completion = func(result int, cop *Operator, err error) {
		completeSendfile(result, cop, err)
		runtime.KeepAlive(op)
	}
	// src
	src, openErr := syscall.Open(filepath, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NONBLOCK|syscall.O_NDELAY, 0777)
	if openErr != nil {
		cb(-1, Userdata{}, os.NewSyscallError("open", openErr))
		return
	}
	curpos, seekToCurrentErr := syscall.Seek(src, 0, io.SeekCurrent)
	if seekToCurrentErr != nil {
		_ = syscall.Close(src)
		cb(-1, Userdata{}, os.NewSyscallError("seek", seekToCurrentErr))
		return
	}
	remain, seekToEndErr := syscall.Seek(src, -curpos, io.SeekEnd)
	if seekToEndErr != nil {
		_ = syscall.Close(src)
		cb(-1, Userdata{}, os.NewSyscallError("seek", seekToEndErr))
		return
	}
	if remain == 0 {
		// empty
		cb(-1, Userdata{}, nil)
		return
	}
	// now seek back to the original position.
	if _, seekToStart := syscall.Seek(src, curpos, io.SeekStart); seekToStart != nil {
		_ = syscall.Close(src)
		cb(-1, Userdata{}, os.NewSyscallError("seek", seekToStart))
		return
	}
	srcFd := &fileFd{
		handle: src,
		path:   filepath,
		rop:    Operator{},
		wop:    Operator{},
	}
	srcFd.rop.fd = srcFd
	srcFd.wop.fd = srcFd

	op.completion = completeSendfile
	op.callback = cb

	op.userdata.Fd = srcFd
	op.userdata.Msg.Controllen = uint32(curpos)
	op.userdata.Msg.Namelen = uint32(remain)

	// prepare write
	cylinder := nextKqueueCylinder()
	if err := cylinder.prepareWrite(fd.Fd(), op); err != nil {
		cb(-1, Userdata{}, err)
		// reset
		op.callback = nil
		op.completion = nil
	}
	return
}

const maxSendfileSize int = 4 << 20

func completeSendfile(result int, op *Operator, err error) {
	dst := op.fd.Fd()
	src := op.userdata.Fd.Fd()
	written := int64(op.userdata.QTY)
	op.userdata.QTY = 0
	curpos := int64(op.userdata.Msg.Controllen)
	op.userdata.Msg.Controllen = 0
	remain := int64(op.userdata.Msg.Namelen)
	op.userdata.Msg.Iovlen = 0

	if err != nil {
		_ = syscall.Close(src)
		op.callback(result, op.userdata, err)
		return
	}

	for remain > 0 {
		n := maxSendfileSize
		if int64(n) > remain {
			n = int(remain)
		}

		pos := curpos
		n, err = syscall.Sendfile(dst, src, &pos, n)
		if n > 0 {
			curpos += int64(n)
			written += int64(n)
			remain -= int64(n)
		}
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		// This includes syscall.ENOSYS (no kernel
		// support) and syscall.EINVAL (fd types which
		// don't implement sendfile), and other errors.
		// We should end the loop when there is no error
		// returned from sendfile(2) or it is not a retryable error.
		if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.EINVAL) {
			break
		}

	}

	_ = syscall.Close(src)

	op.userdata.QTY = uint32(written)
	op.callback(int(written), op.userdata, os.NewSyscallError("sendfile", err))
	return
}
