//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"github.com/brickingsoft/errors"
	"io"
	"os"
	"syscall"
)

func Sendfile(fd NetFd, filepath string, cb OperationCallback) {
	if len(filepath) == 0 {
		err := errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(errors.Define("filepath is empty")),
		)
		cb(Userdata{}, err)
		return
	}

	// src
	src, openErr := syscall.Open(filepath, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NONBLOCK|syscall.O_NDELAY, 0777)
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
	curpos, seekToCurrentErr := syscall.Seek(src, 0, io.SeekCurrent)
	if seekToCurrentErr != nil {
		_ = syscall.Close(src)

		err := errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(os.NewSyscallError("seek", seekToCurrentErr)),
		)
		cb(Userdata{}, err)
		return
	}
	remain, seekToEndErr := syscall.Seek(src, -curpos, io.SeekEnd)
	if seekToEndErr != nil {
		_ = syscall.Close(src)
		err := errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(os.NewSyscallError("seek", seekToEndErr)),
		)
		cb(Userdata{}, err)
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
		err := errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(os.NewSyscallError("seek", seekToStart)),
		)
		cb(Userdata{}, err)
		return
	}

	// op
	op := acquireOperator(fd)
	if setOp := fd.SetWOP(op); !setOp {
		_ = syscall.Close(src)
		releaseOperator(op)
		err := errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(errors.From(ErrRepeatOperation)),
		)
		cb(Userdata{}, err)
		return
	}
	op.callback = cb
	op.completion = completeSendfile
	op.sfr.file = src
	op.sfr.curpos = curpos
	op.sfr.remain = remain
	op.sfr.written = 0

	// prepare write
	cylinder := fd.Cylinder().(*KqueueCylinder)
	if err := cylinder.prepareWrite(fd.Fd(), op); err != nil {
		fd.RemoveWOP()
		releaseOperator(op)
		err = errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(err),
		)
		cb(Userdata{}, err)
	}
	return
}

const maxSendfileSize int = 4 << 20

func completeSendfile(result int, op *Operator, err error) {
	cb := op.callback
	fd := op.fd
	dst := fd.Fd()
	src := op.sfr.file
	written := op.sfr.written
	curpos := op.sfr.curpos
	remain := op.sfr.remain
	fd.RemoveWOP()
	releaseOperator(op)

	if err != nil {
		_ = syscall.Close(src)
		err = errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(err),
		)
		cb(Userdata{N: written}, err)
		return
	}

	if result == 0 {
		_ = syscall.Close(src)
		cb(Userdata{N: written}, nil)
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
	if err != nil {
		err = errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(os.NewSyscallError("sendfile", err)),
		)
	}
	cb(Userdata{N: written}, err)
	return
}
