//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"errors"
	"io"
	"os"
	"syscall"
)

func Sendfile(fd NetFd, filepath string, cb OperationCallback) {
	// op
	op := fd.WriteOperator()
	if len(filepath) == 0 {
		cb(Userdata{}, errors.New("aio.Sendfile: filepath is empty"))
		return
	}
	op.callback = cb
	op.completion = completeSendfile
	// src
	src, openErr := syscall.Open(filepath, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NONBLOCK|syscall.O_NDELAY, 0777)
	if openErr != nil {
		cb(Userdata{}, os.NewSyscallError("open", openErr))
		return
	}
	curpos, seekToCurrentErr := syscall.Seek(src, 0, io.SeekCurrent)
	if seekToCurrentErr != nil {
		_ = syscall.Close(src)
		cb(Userdata{}, os.NewSyscallError("seek", seekToCurrentErr))
		return
	}
	remain, seekToEndErr := syscall.Seek(src, -curpos, io.SeekEnd)
	if seekToEndErr != nil {
		_ = syscall.Close(src)
		cb(Userdata{}, os.NewSyscallError("seek", seekToEndErr))
		return
	}
	if remain == 0 {
		// empty
		cb(Userdata{}, nil)
		return
	}
	// now seek back to the original position.
	if _, seekToStart := syscall.Seek(src, curpos, io.SeekStart); seekToStart != nil {
		_ = syscall.Close(src)
		cb(Userdata{}, os.NewSyscallError("seek", seekToStart))
		return
	}
	op.sfr = &SendfileResult{
		file:    src,
		curpos:  curpos,
		remain:  remain,
		written: 0,
	}

	op.completion = completeSendfile
	op.callback = cb

	// prepare write
	cylinder := nextKqueueCylinder()
	op.setCylinder(cylinder)

	if err := cylinder.prepareWrite(fd.Fd(), op); err != nil {
		cb(Userdata{}, err)
		// reset
		op.reset()
	}
	return
}

const maxSendfileSize int = 4 << 20

func completeSendfile(result int, op *Operator, err error) {
	dst := op.fd.Fd()
	src := op.sfr.file
	written := op.sfr.written
	curpos := op.sfr.curpos
	remain := op.sfr.remain

	if err != nil {
		_ = syscall.Close(src)
		op.callback(Userdata{N: written}, err)
		return
	}

	if result == 0 {
		_ = syscall.Close(src)
		op.callback(Userdata{N: written}, err)
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
			written += n
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
	op.callback(Userdata{N: written}, os.NewSyscallError("sendfile", err))
	return
}
