//go:build linux

package aio

import (
	"github.com/brickingsoft/errors"
	"io"
	"os"
	"syscall"
	"unsafe"
)

func Recv(fd NetFd, b []byte, cb OperationCallback) {
	// op
	op := acquireOperator(fd)
	if setOp := fd.SetROP(op); !setOp {
		releaseOperator(op)
		err := errors.New(
			"receive failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpRecv),
			errors.WithWrap(errors.From(ErrRepeatOperation)),
		)
		cb(Userdata{}, err)
		return
	}
	// cb
	op.callback = cb
	// completion
	op.completion = completeRecv

	// msg
	bufAddr := uintptr(unsafe.Pointer(&b[0]))
	bufLen := uint32(len(b))

	// prepare
	cylinder := fd.Cylinder().(*IOURingCylinder)
	err := cylinder.prepareRW(opRecv, fd.Fd(), bufAddr, bufLen, 0, 0, op.ptr())
	if err != nil {
		fd.RemoveROP()
		releaseOperator(op)
		err = errors.New(
			"receive failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpRecv),
			errors.WithWrap(os.NewSyscallError("io_uring_prep_recv", err)),
		)
		cb(Userdata{}, err)
		return
	}
	return
}

func completeRecv(result int, op *Operator, err error) {
	cb := op.callback
	fd := op.fd
	fd.RemoveROP()
	releaseOperator(op)
	if err != nil {
		err = errors.New(
			"receive failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpRecv),
			errors.WithWrap(err),
		)
		cb(Userdata{}, err)
		return
	}
	if result == 0 && fd.ZeroReadIsEOF() {
		cb(Userdata{}, io.EOF)
		return
	}
	cb(Userdata{N: result}, nil)
	return
}

func RecvFrom(fd NetFd, b []byte, cb OperationCallback) {
	RecvMsg(fd, b, nil, cb)
	return
}

func RecvMsg(fd NetFd, b []byte, oob []byte, cb OperationCallback) {
	// op
	op := acquireOperator(fd)
	if setOp := fd.SetROP(op); !setOp {
		releaseOperator(op)
		err := errors.New(
			"receive message failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpRecvMsg),
			errors.WithWrap(errors.From(ErrRepeatOperation)),
		)
		cb(Userdata{}, err)
		return
	}
	// cb
	op.callback = cb
	// completion
	op.completion = completeRecvMsg
	// msg
	op.msg.Name = (*byte)(unsafe.Pointer(new(syscall.RawSockaddrAny)))
	op.msg.Namelen = syscall.SizeofSockaddrAny

	bLen := len(b)
	if bLen > 0 {
		op.msg.Iov = &syscall.Iovec{
			Base: &b[0],
			Len:  uint64(bLen),
		}
		op.msg.Iovlen = 1
	}
	if oobLen := len(oob); oobLen > 0 {
		op.msg.Control = &oob[0]
		op.msg.Controllen = uint64(oobLen)
		if bLen == 0 && fd.SocketType() != syscall.SOCK_DGRAM {
			var dummy byte
			op.msg.Iov = &syscall.Iovec{
				Base: &dummy,
				Len:  uint64(1),
			}
			op.msg.Iovlen = 1
		}
	}
	if fd.Family() == syscall.AF_UNIX {
		op.msg.Flags |= readMsgFlags
	}

	// prepare
	cylinder := fd.Cylinder().(*IOURingCylinder)
	err := cylinder.prepareRW(opRecvmsg, fd.Fd(), uintptr(unsafe.Pointer(&op.msg)), uint32(op.msg.Iovlen), 0, 0, op.ptr())
	if err != nil {
		fd.RemoveROP()
		releaseOperator(op)
		err = errors.New(
			"receive message failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpRecvMsg),
			errors.WithWrap(os.NewSyscallError("io_uring_prep_recvmsg", err)),
		)
		cb(Userdata{}, err)
		return
	}
	return
}

func completeRecvMsg(result int, op *Operator, err error) {
	cb := op.callback
	msg := op.msg
	fd := op.fd
	fd.RemoveROP()
	releaseOperator(op)
	if err != nil {
		err = errors.New(
			"receive message failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpRecvMsg),
			errors.WithWrap(err),
		)
		cb(Userdata{}, err)
		return
	}
	rsa := (*syscall.RawSockaddrAny)(unsafe.Pointer(msg.Name))
	addr, addrErr := RawToAddr(rsa)
	if addrErr != nil {
		err = errors.New(
			"receive message failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpRecvMsg),
			errors.WithWrap(addrErr),
		)
		cb(Userdata{}, err)
		return
	}
	oobn := int(msg.Controllen)
	flags := int(msg.Flags)
	cb(Userdata{N: result, OOBN: oobn, Addr: addr, MessageFlags: flags}, nil)
	return
}
