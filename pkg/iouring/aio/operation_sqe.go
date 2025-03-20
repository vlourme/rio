//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/iouring"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

func (op *Operation) makeSQE(r *Ring) error {
	sqe := r.ring.GetSQE()
	if sqe == nil {
		return os.NewSyscallError("ring_getsqe", syscall.EBUSY)
	}
	switch op.kind {
	case iouring.OpNop:
		sqe.PrepareNop()
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSocket:
		family := op.pipe.fdIn
		sotype := op.pipe.fdOut
		proto := int(op.pipe.offIn)
		if op.directMode {
			sqe.PrepareSocketDirectAlloc(family, sotype, proto, 0)
		} else {
			sqe.PrepareSocket(family, sotype, proto, 0)
		}
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpUringCmd:
		fd := op.fd
		level := op.pipe.fdIn
		optName := op.pipe.fdOut
		switch uint64(op.subKind) {
		case iouring.SocketOpGetsockopt:
			var optValue = (*int)(op.ptr)
			sqe.PrepareGetsockoptInt(fd, level, optName, optValue)
		case iouring.SocketOpSetsockopt:
			optValue := int(op.pipe.offIn)
			sqe.PrepareSetsockoptInt(fd, level, optName, &optValue)
		default:
			sqe.PrepareNop()
			return ErrUnsupportedOp
		}
		sqe.SetFlags(op.sqeFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpConnect:
		addrPtr := (*syscall.RawSockaddrAny)(unsafe.Pointer(op.msg.Name))
		addrLenPtr := uint64(op.msg.Namelen)
		sqe.PrepareConnect(op.fd, addrPtr, addrLenPtr)
		sqe.SetFlags(op.sqeFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpAccept:
		addrPtr := (*syscall.RawSockaddrAny)(unsafe.Pointer(op.msg.Name))
		addrLenPtr := uint64(uintptr(unsafe.Pointer(&op.msg.Namelen)))
		if op.multishot {
			if op.directMode {
				sqe.PrepareAcceptMultishotDirect(op.fd, addrPtr, addrLenPtr, syscall.SOCK_NONBLOCK)
			} else {
				sqe.PrepareAcceptMultishot(op.fd, addrPtr, addrLenPtr, syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC)
			}
		} else {
			if op.directMode && op.filedIndex > -1 {
				sqe.PrepareAcceptDirect(op.fd, addrPtr, addrLenPtr, syscall.SOCK_NONBLOCK, uint32(op.filedIndex))
			} else {
				sqe.PrepareAccept(op.fd, addrPtr, addrLenPtr, syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC)
			}
		}
		sqe.SetFlags(op.sqeFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpClose:
		if op.directMode && op.filedIndex > -1 {
			sqe.PrepareCloseDirect(uint32(op.filedIndex))
		} else {
			sqe.PrepareClose(op.fd)
		}
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpReadFixed:
		b := uintptr(unsafe.Pointer(op.msg.Name))
		bLen := op.msg.Namelen
		idx := op.msg.Iovlen
		sqe.PrepareReadFixed(op.fd, b, bLen, 0, int(idx))
		sqe.SetFlags(op.sqeFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpWriteFixed:
		b := uintptr(unsafe.Pointer(op.msg.Name))
		bLen := op.msg.Namelen
		idx := op.msg.Iovlen
		sqe.PrepareWriteFixed(op.fd, b, bLen, 0, int(idx))
		sqe.SetFlags(op.sqeFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpRecv:
		b := uintptr(unsafe.Pointer(op.msg.Name))
		bLen := op.msg.Namelen
		sqe.PrepareRecv(op.fd, b, bLen, 0)
		sqe.SetFlags(op.sqeFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSend:
		b := uintptr(unsafe.Pointer(op.msg.Name))
		bLen := op.msg.Namelen
		sqe.PrepareSend(op.fd, b, bLen, 0)
		sqe.SetFlags(op.sqeFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSendZC:
		b := uintptr(unsafe.Pointer(op.msg.Name))
		bLen := op.msg.Namelen
		sqe.PrepareSendZC(op.fd, b, bLen, 0, 0)
		sqe.SetFlags(op.sqeFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpRecvmsg:
		sqe.PrepareRecvMsg(op.fd, &op.msg, 0)
		sqe.SetFlags(op.sqeFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSendmsg:
		sqe.PrepareSendMsg(op.fd, &op.msg, 0)
		sqe.SetFlags(op.sqeFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSendMsgZC:
		sqe.PrepareSendmsgZC(op.fd, &op.msg, 0)
		sqe.SetFlags(op.sqeFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSplice:
		sqe.PrepareSplice(op.pipe.fdIn, op.pipe.offIn, op.pipe.fdOut, op.pipe.offOut, op.pipe.nbytes, op.pipe.spliceFlags)
		sqe.SetFlags(op.sqeFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpTee:
		sqe.PrepareTee(op.pipe.fdIn, op.pipe.fdOut, op.pipe.nbytes, op.pipe.spliceFlags)
		sqe.SetFlags(op.sqeFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpAsyncCancel:
		if op.ptr != nil {
			sqe.PrepareCancel(uintptr(op.ptr), 0)
			sqe.SetFlags(op.sqeFlags)
			sqe.SetData(unsafe.Pointer(op))
		} else if op.fd != -1 || op.directMode {
			if op.directMode && op.filedIndex > -1 {
				sqe.PrepareCancelFdFixed(uint32(op.filedIndex), 0)
			} else {
				sqe.PrepareCancelFd(op.fd, 0)
			}
			sqe.SetFlags(op.sqeFlags)
			sqe.SetData(unsafe.Pointer(op))
		} else {
			sqe.PrepareNop()
			return ErrUnsupportedOp
		}
		break
	case iouring.OPFixedFdInstall:
		sqe.PrepareFixedFdInstall(op.fd, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpShutdown:
		sqe.PrepareShutdown(op.fd, op.pipe.fdIn)
		sqe.SetFlags(op.sqeFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	default:
		sqe.PrepareNop()
		return ErrUnsupportedOp
	}
	runtime.KeepAlive(sqe)
	return nil
}
