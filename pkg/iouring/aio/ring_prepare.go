//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/process"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

func (r *Ring) preparingSQE(ctx context.Context) {
	defer r.wg.Done()

	// cpu affinity
	if r.ring.Flags()&iouring.SetupSingleIssuer != 0 {
		runtime.LockOSThread()
		if setErr := process.SetCPUAffinity(r.id); setErr != nil {
			runtime.UnlockOSThread()
		} else {
			defer runtime.UnlockOSThread()
		}
	}

	ring := r.ring

	queue := r.requests
	shs := r.submitSemaphores
	var (
		peeked       uint32
		prepared     uint32
		needToSubmit int64
	)

	prepareBatch := r.prepSQEBatchSize
	if prepareBatch < 1 {
		prepareBatch = 1024
	}
	operations := make([]*Operation, prepareBatch)
	for {
		if peeked = queue.PeekBatch(operations); peeked == 0 {
			if needToSubmit > 0 {
				submitted, _ := ring.Submit()
				needToSubmit -= int64(submitted)
			}
			if waitErr := shs.Wait(ctx); waitErr != nil {
				if errors.Is(waitErr, context.Canceled) {
					break
				}
			}
			continue
		}
		for i := uint32(0); i < peeked; i++ {
			op := operations[i]
			if op == nil {
				continue
			}
			operations[i] = nil
			if op.canPrepare() {
				if prepErr := r.prepareSQE(op); prepErr != nil { // when prep err occur, means invalid op kind or no sqe left
					op.setResult(0, 0, prepErr)
					if errors.Is(prepErr, syscall.EBUSY) { // no sqe left
						break
					}
					prepared++ // prepareSQE nop whit out userdata, so prepared++
					continue
				}
				prepared++
			}
		}
		// submit
		if prepared > 0 || needToSubmit > 0 {
			submitted, _ := ring.Submit()
			needToSubmit += int64(prepared) - int64(submitted)
			r.requests.Advance(prepared)
			prepared = 0
		}
	}
	// evict
	if remains := r.requests.Length(); remains > 0 {
		peeked = r.requests.PeekBatch(operations)
		for i := uint32(0); i < peeked; i++ {
			op := operations[i]
			operations[i] = nil
			op.setResult(0, 0, Uncompleted)
		}
	}
	// prepare noop to wakeup cq waiter
	for {
		sqe := ring.GetSQE()
		if sqe == nil {
			time.Sleep(1 * time.Millisecond)
			continue
		}
		sqe.PrepareNop()
		for {
			if _, subErr := ring.Submit(); subErr != nil {
				time.Sleep(1 * time.Millisecond)
				continue
			}
			break
		}
		break
	}
	return
}

func (r *Ring) prepareSQE(op *Operation) error {
	sqe := r.ring.GetSQE()
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
