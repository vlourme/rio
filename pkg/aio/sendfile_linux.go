//go:build linux

package aio

import (
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

func Sendfile(fd NetFd, filepath string, cb OperationCallback) {
	// op
	op := writeOperator(fd)
	if len(filepath) == 0 {
		cb(-1, Userdata{}, errors.New("aio.Sendfile: filepath is empty"))
		return
	}
	op.callback = cb
	op.completion = func(result int, cop *Operator, err error) {
		completeSendfileToPipe(result, cop, err)
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
	op.userdata.Fd = srcFd
	op.userdata.QTY = uint32(remain)

	// pipe
	pipe := make([]int, 2)
	if pipeErr := Pipe2(pipe); pipeErr != nil {
		_ = syscall.Close(src)
		cb(-1, Userdata{}, pipeErr)
		return
	}
	op.userdata.Msg.Iovlen = uint64(pipe[0])
	op.userdata.Msg.Controllen = uint64(pipe[1])

	// cylinder
	cylinder := nextIOURingCylinder()
	entry, getErr := cylinder.getSQE()
	if getErr != nil {
		_ = syscall.Close(src)
		_ = syscall.Close(pipe[0])
		_ = syscall.Close(pipe[1])
		cb(-1, Userdata{}, getErr)

		op.callback = nil
		op.completion = nil
		return
	}
	// userdata
	userdata := uint64(uintptr(unsafe.Pointer(op)))
	entry.PrepareSplice(src, -1, pipe[1], -1, op.userdata.QTY, unix.SPLICE_F_NONBLOCK, userdata)
	runtime.KeepAlive(op)
}

func completeSendfileToPipe(result int, op *Operator, err error) {
	// src
	src := op.userdata.Fd.Fd()
	// pipe
	pipe := []int{
		int(op.userdata.Msg.Iovlen),
		int(op.userdata.Msg.Controllen),
	}
	if err != nil {
		_ = syscall.Close(src)
		_ = syscall.Close(pipe[0])
		_ = syscall.Close(pipe[1])
		op.callback(-1, Userdata{}, err)
		return
	}
	nop := writeOperator(op.fd)
	nop.userdata.Fd = op.userdata.Fd                         // src
	nop.userdata.QTY = op.userdata.QTY                       // size
	nop.userdata.Msg.Iovlen = op.userdata.Msg.Iovlen         // pipe 0
	nop.userdata.Msg.Controllen = op.userdata.Msg.Controllen // pipe 1
	nop.callback = op.callback
	nop.completion = func(result int, cop *Operator, err error) {
		completeSendfileFromPipe(result, cop, err)
		runtime.KeepAlive(nop)
	}
	// dst
	dst := nop.fd.Fd()

	// size
	size := nop.userdata.QTY

	// cylinder
	cylinder := nextIOURingCylinder()
	entry, getErr := cylinder.getSQE()
	if getErr != nil {
		_ = syscall.Close(src)
		_ = syscall.Close(pipe[0])
		_ = syscall.Close(pipe[1])
		nop.callback(-1, Userdata{}, getErr)
		return
	}

	// userdata
	userdata := uint64(uintptr(unsafe.Pointer(nop)))
	entry.PrepareSplice(pipe[0], -1, dst, -1, size, unix.SPLICE_F_NONBLOCK, userdata)
	runtime.KeepAlive(nop)
	return
}

func completeSendfileFromPipe(result int, op *Operator, err error) {
	// src
	src := op.userdata.Fd.Fd()
	_ = syscall.Close(src)
	// pipe
	pipe := []int{
		int(op.userdata.Msg.Iovlen),
		int(op.userdata.Msg.Controllen),
	}
	_ = syscall.Close(pipe[0])
	_ = syscall.Close(pipe[1])
	// handle
	if err != nil {
		op.callback(-1, Userdata{}, err)
		return
	}
	op.callback(result, op.userdata, nil)
	return
}
