package aio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

func (vortex *Vortex) PrepareOperation(ctx context.Context, op *Operation) Future {
	stopped := false
	for {
		select {
		case <-ctx.Done():
			return Future{err: ctx.Err()}
		default:
			if pushed := vortex.queue.Enqueue(op); pushed {
				stopped = true
			}
			break
		}
		if stopped {
			break
		}
	}
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareAccept(ctx context.Context, fd int, addr *syscall.RawSockaddrAny, addrLen int) Future {
	op := vortex.acquireOperation()
	op.PrepareAccept(fd, addr, addrLen)
	return vortex.prepareOperation(ctx, op)
}

func (vortex *Vortex) PrepareReceive(ctx context.Context, fd int, b []byte, timeout time.Duration) Future {
	op := vortex.acquireOperation()
	op.WithTimeout(timeout).PrepareReceive(fd, b)
	return vortex.prepareOperation(ctx, op)
}

func (vortex *Vortex) PrepareSend(ctx context.Context, fd int, b []byte, timeout time.Duration) Future {
	op := vortex.acquireOperation()
	op.WithTimeout(timeout).PrepareSend(fd, b)
	return vortex.prepareOperation(ctx, op)
}

func (vortex *Vortex) PrepareSendZC(ctx context.Context, fd int, b []byte, timeout time.Duration) Future {
	op := vortex.acquireOperation()
	op.WithTimeout(timeout).PrepareSendZC(fd, b)
	return vortex.prepareOperation(ctx, op)
}

func (vortex *Vortex) PrepareReceiveMsg(ctx context.Context, fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32, timeout time.Duration) Future {
	op := vortex.acquireOperation()
	op.WithTimeout(timeout).PrepareReceiveMsg(fd, b, oob, addr, addrLen, flags)
	return vortex.prepareOperation(ctx, op)
}

func (vortex *Vortex) PrepareSendMsg(ctx context.Context, fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32, timeout time.Duration) Future {
	op := vortex.acquireOperation()
	op.WithTimeout(timeout).PrepareSendMsg(fd, b, oob, addr, addrLen, flags)
	return vortex.prepareOperation(ctx, op)
}

func (vortex *Vortex) PrepareSendMsgZC(ctx context.Context, fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32, timeout time.Duration) Future {
	op := vortex.acquireOperation()
	op.WithTimeout(timeout).PrepareSendMsgZC(fd, b, oob, addr, addrLen, flags)
	return vortex.prepareOperation(ctx, op)
}

func (vortex *Vortex) PrepareSplice(ctx context.Context, fdIn int, offIn int64, fdOut int, offOut int64, nbytes uint32, flags uint32) Future {
	op := vortex.acquireOperation()
	op.PrepareSplice(fdIn, offIn, fdOut, offOut, nbytes, flags)
	return vortex.prepareOperation(ctx, op)
}

func (vortex *Vortex) PrepareTee(ctx context.Context, fdIn int, fdOut int, nbytes uint32, flags uint32) Future {
	op := vortex.acquireOperation()
	op.PrepareTee(fdIn, fdOut, nbytes, flags)
	return vortex.prepareOperation(ctx, op)
}

const (
	ns500 = 500 * time.Nanosecond
)

func (vortex *Vortex) prepareOperation(ctx context.Context, op *Operation) Future {
	for {
		select {
		case <-ctx.Done():
			vortex.releaseOperation(op)
			return Future{err: ctx.Err()}
		default:
			if pushed := vortex.queue.Enqueue(op); pushed {
				return Future{
					vortex:   vortex,
					op:       op,
					acquired: true,
				}
			}
			time.Sleep(ns500)
			break
		}
	}
}

func (vortex *Vortex) prepareSQE(op *Operation) (bool, error) {
	sqe := vortex.ring.GetSQE()
	if sqe == nil {
		return false, nil
	}
	switch op.kind {
	case iouring.OpNop:
		sqe.PrepareNop()
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpAccept:
		addrPtr := uintptr(unsafe.Pointer(op.msg.Name))
		addrLenPtr := uint64(uintptr(unsafe.Pointer(&op.msg.Namelen)))
		sqe.PrepareAccept(op.fd, addrPtr, addrLenPtr, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpRecv:
		b := uintptr(unsafe.Pointer(&op.b[0]))
		bLen := uint32(len(op.b))
		sqe.PrepareRecv(op.fd, b, bLen, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSend:
		b := uintptr(unsafe.Pointer(&op.b[0]))
		bLen := uint32(len(op.b))
		sqe.PrepareSend(op.fd, b, bLen, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSendZC:
		sqe.PrepareSendZC(op.fd, op.b, 0, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpRecvmsg:
		msg := op.msg
		sqe.PrepareRecvMsg(op.fd, &msg, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSendmsg:
		msg := op.msg
		sqe.PrepareSendMsg(op.fd, &msg, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSendMsgZC:
		msg := op.msg
		sqe.PrepareSendmsgZC(op.fd, &msg, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSplice:
		sp := op.pipe
		sqe.PrepareSplice(sp.fdIn, sp.offIn, sp.fdOut, sp.offOut, sp.nbytes, sp.spliceFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpTee:
		sp := op.pipe
		sqe.PrepareTee(sp.fdIn, sp.fdOut, sp.nbytes, sp.spliceFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpAsyncCancel:
		ptr := op.ptr
		sqe.PrepareCancel(uintptr(ptr), 0)
		break
	default:
		sqe.PrepareNop()
		return false, UnsupportedOp
	}
	runtime.KeepAlive(sqe)
	return true, nil
}
