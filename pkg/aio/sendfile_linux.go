//go:build linux

package aio

import (
	"github.com/brickingsoft/errors"
	"golang.org/x/sys/unix"
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
			errors.WithWrap(errors.Define("aio.Sendfile: filepath is empty")),
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
	// now seek back to the original position.
	if _, seekToStart := syscall.Seek(src, curpos, io.SeekStart); seekToStart != nil {
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

	// pipe
	pipe := make([]int, 2)
	if pipeErr := Pipe2(pipe); pipeErr != nil {
		_ = syscall.Close(src)
		err := errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(pipeErr),
		)
		cb(Userdata{}, err)
		return
	}

	// op
	op := acquireOperator(fd)
	if setOp := fd.SetWOP(op); !setOp {
		releaseOperator(op)
		_ = syscall.Close(src)
		_ = syscall.Close(pipe[0])
		_ = syscall.Close(pipe[1])
		err := errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(errors.From(ErrRepeatOperation)),
		)
		cb(Userdata{}, err)
		return
	}
	// callback
	op.callback = cb
	// completion
	op.completion = completeSendfileToPipe
	// result
	op.sfr.file = src
	op.sfr.remain = uint32(remain)
	op.sfr.pipe = pipe

	// prepare
	cylinder := fd.Cylinder().(*IOURingCylinder)
	entry, getErr := cylinder.getSQE()
	if getErr != nil {
		fd.RemoveWOP()
		releaseOperator(op)
		_ = syscall.Close(src)
		_ = syscall.Close(pipe[0])
		_ = syscall.Close(pipe[1])
		err := errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(getErr),
		)
		cb(Userdata{}, err)

		return
	}
	// splice
	prepareSplice(entry, src, -1, pipe[1], -1, op.sfr.remain, unix.SPLICE_F_NONBLOCK, op.ptr())
}

func completeSendfileToPipe(_ int, op *Operator, err error) {
	cb := op.callback
	fd := op.fd

	// src
	src := op.sfr.file
	// pipe
	pipe := op.sfr.pipe

	if err != nil {
		fd.RemoveWOP()
		releaseOperator(op)

		_ = syscall.Close(src)
		_ = syscall.Close(pipe[0])
		_ = syscall.Close(pipe[1])

		err = errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(err),
		)
		cb(Userdata{}, err)
		return
	}
	op.completion = completeSendfileFromPipe
	dst := fd.Fd()
	size := op.sfr.remain

	// prepare
	cylinder := fd.Cylinder().(*IOURingCylinder)
	entry, getErr := cylinder.getSQE()
	if getErr != nil {
		fd.RemoveWOP()
		releaseOperator(op)

		_ = syscall.Close(src)
		_ = syscall.Close(pipe[0])
		_ = syscall.Close(pipe[1])

		err = errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(getErr),
		)
		cb(Userdata{}, err)
		return
	}
	// splice
	prepareSplice(entry, pipe[0], -1, dst, -1, size, unix.SPLICE_F_NONBLOCK, op.ptr())
	return
}

func completeSendfileFromPipe(result int, op *Operator, err error) {
	cb := op.callback
	sfr := op.sfr
	fd := op.fd
	fd.RemoveWOP()
	releaseOperator(op)
	// src
	_ = syscall.Close(sfr.file)
	// pipe
	pipe := sfr.pipe
	_ = syscall.Close(pipe[0])
	_ = syscall.Close(pipe[1])
	// handle
	if err != nil {
		err = errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendfile),
			errors.WithWrap(err),
		)
		cb(Userdata{}, err)
		return
	}
	cb(Userdata{N: result}, nil)
	return
}

func prepareSplice(entry *SubmissionQueueEntry, fdIn int, offIn int64, fdOut int, offOut int64, nbytes uint32, spliceFlags uint32, userdata uint64) {
	entry.prepareRW(opSplice, fdOut, 0, nbytes, uint64(offOut), userdata, 0)
	entry.Addr = uint64(offIn)
	entry.SpliceFdIn = int32(fdIn)
	entry.OpcodeFlags = spliceFlags
}
