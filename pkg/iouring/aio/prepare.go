//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/iouring"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

func (vortex *Vortex) PrepareOperation(op *Operation) Future {
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareConnect(fd int, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareConnect(fd, addr, addrLen)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareAccept(fd int, addr *syscall.RawSockaddrAny, addrLen int) Future {
	op := vortex.acquireOperation()
	op.PrepareAccept(fd, addr, addrLen)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareAcceptMultishot(fd int, addr *syscall.RawSockaddrAny, addrLen int, buffer int) Future {
	op := NewOperation(buffer)
	op.Hijack()
	op.PrepareAcceptMultishot(fd, addr, addrLen)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareClose(fd int) Future {
	op := vortex.acquireOperation()
	op.PrepareClose(fd)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareReadFixed(fd int, buf *FixedBuffer, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReadFixed(fd, buf)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareWriteFixed(fd int, buf *FixedBuffer, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareWriteFixed(fd, buf)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareReceive(fd int, b []byte, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReceive(fd, b)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareSend(fd int, b []byte, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSend(fd, b)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareSendZC(fd int, b []byte, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendZC(fd, b)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareReceiveMsg(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReceiveMsg(fd, b, oob, addr, addrLen, flags)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareSendMsg(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendMsg(fd, b, oob, addr, addrLen, flags)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareSendMsgZC(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendMsgZC(fd, b, oob, addr, addrLen, flags)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareSplice(fdIn int, offIn int64, fdOut int, offOut int64, nbytes uint32, flags uint32) Future {
	op := vortex.acquireOperation()
	op.PrepareSplice(fdIn, offIn, fdOut, offOut, nbytes, flags)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareTee(fdIn int, fdOut int, nbytes uint32, flags uint32) Future {
	op := vortex.acquireOperation()
	op.PrepareTee(fdIn, fdOut, nbytes, flags)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) prepareSQE(op *Operation) error {
	sqe := vortex.ring.GetSQE()
	if sqe == nil {
		return os.NewSyscallError("ring_getsqe", syscall.EBUSY)
	}
	switch op.kind {
	case iouring.OpNop:
		sqe.PrepareNop()
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpConnect:
		addrPtr := (*syscall.RawSockaddrAny)(unsafe.Pointer(op.msg.Name))
		addrLenPtr := uint64(op.msg.Namelen)
		sqe.PrepareConnect(op.fd, addrPtr, addrLenPtr)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpAccept:
		addrPtr := (*syscall.RawSockaddrAny)(unsafe.Pointer(op.msg.Name))
		addrLenPtr := uint64(uintptr(unsafe.Pointer(&op.msg.Namelen)))
		if op.multishot {
			sqe.PrepareAcceptMultishot(op.fd, addrPtr, addrLenPtr, 0)
		} else {
			sqe.PrepareAccept(op.fd, addrPtr, addrLenPtr, 0)
		}
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpClose:
		sqe.PrepareClose(op.fd)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpReadFixed:
		b := uintptr(unsafe.Pointer(op.msg.Name))
		bLen := op.msg.Namelen
		idx := op.msg.Iovlen
		sqe.PrepareReadFixed(op.fd, b, bLen, 0, int(idx))
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpWriteFixed:
		b := uintptr(unsafe.Pointer(op.msg.Name))
		bLen := op.msg.Namelen
		idx := op.msg.Iovlen
		sqe.PrepareWriteFixed(op.fd, b, bLen, 0, int(idx))
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpRecv:
		b := uintptr(unsafe.Pointer(op.msg.Name))
		bLen := op.msg.Namelen
		sqe.PrepareRecv(op.fd, b, bLen, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSend:
		b := uintptr(unsafe.Pointer(op.msg.Name))
		bLen := op.msg.Namelen
		sqe.PrepareSend(op.fd, b, bLen, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSendZC:
		b := uintptr(unsafe.Pointer(op.msg.Name))
		bLen := op.msg.Namelen
		sqe.PrepareSendZC(op.fd, b, bLen, 0, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpRecvmsg:
		sqe.PrepareRecvMsg(op.fd, &op.msg, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSendmsg:
		sqe.PrepareSendMsg(op.fd, &op.msg, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSendMsgZC:
		sqe.PrepareSendmsgZC(op.fd, &op.msg, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSplice:
		sqe.PrepareSplice(op.pipe.fdIn, op.pipe.offIn, op.pipe.fdOut, op.pipe.offOut, op.pipe.nbytes, op.pipe.spliceFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpTee:
		sqe.PrepareTee(op.pipe.fdIn, op.pipe.fdOut, op.pipe.nbytes, op.pipe.spliceFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpAsyncCancel:
		sqe.PrepareCancel(uintptr(op.ptr), 0)
		break
	default:
		sqe.PrepareNop()
		return UnsupportedOp
	}
	runtime.KeepAlive(sqe)
	return nil
}
