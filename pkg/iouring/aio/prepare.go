package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

func (vortex *Vortex) PrepareOperation(ctx context.Context, op *Operation) Future {
	return vortex.prepareOperation(ctx, op)
}

func (vortex *Vortex) PrepareConnect(ctx context.Context, fd int, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareConnect(fd, addr, addrLen)
	return vortex.prepareOperation(ctx, op)
}

func (vortex *Vortex) PrepareAccept(ctx context.Context, fd int, addr *syscall.RawSockaddrAny, addrLen int) Future {
	op := vortex.acquireOperation()
	op.PrepareAccept(fd, addr, addrLen)
	return vortex.prepareOperation(ctx, op)
}

func (vortex *Vortex) PrepareReceive(ctx context.Context, fd int, b []byte, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReceive(fd, b)
	return vortex.prepareOperation(ctx, op)
}

func (vortex *Vortex) PrepareSend(ctx context.Context, fd int, b []byte, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSend(fd, b)
	return vortex.prepareOperation(ctx, op)
}

func (vortex *Vortex) PrepareSendZC(ctx context.Context, fd int, b []byte, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendZC(fd, b)
	return vortex.prepareOperation(ctx, op)
}

func (vortex *Vortex) PrepareReceiveMsg(ctx context.Context, fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReceiveMsg(fd, b, oob, addr, addrLen, flags)
	return vortex.prepareOperation(ctx, op)
}

func (vortex *Vortex) PrepareSendMsg(ctx context.Context, fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendMsg(fd, b, oob, addr, addrLen, flags)
	return vortex.prepareOperation(ctx, op)
}

func (vortex *Vortex) PrepareSendMsgZC(ctx context.Context, fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendMsgZC(fd, b, oob, addr, addrLen, flags)
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
	if timeout := op.Timeout(ctx); timeout > 0 {
		timer := vortex.acquireTimer(timeout)
		for {
			select {
			case <-ctx.Done():
				vortex.releaseOperation(op)
				vortex.releaseTimer(timer)
				err := ctx.Err()
				if errors.Is(err, context.DeadlineExceeded) {
					err = Timeout
				}
				return Future{err: ctx.Err()}
			case <-timer.C:
				vortex.releaseOperation(op)
				vortex.releaseTimer(timer)
				return Future{err: Timeout}
			default:
				if pushed := vortex.ops.Submit(op); pushed {
					vortex.releaseTimer(timer)
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
	} else if timeout < 0 {
		return Future{err: Timeout}
	} else {
		for {
			select {
			case <-ctx.Done():
				vortex.releaseOperation(op)
				err := ctx.Err()
				if errors.Is(err, context.DeadlineExceeded) {
					err = Timeout
				}
				return Future{err: err}
			default:
				if pushed := vortex.ops.Submit(op); pushed {
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
		vortex.hijackedOps.Delete(op)
		break
	default:
		sqe.PrepareNop()
		return UnsupportedOp
	}
	runtime.KeepAlive(sqe)
	return nil
}
