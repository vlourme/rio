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
	if r.ring.Flags()&iouring.SetupSingleIssuer != 0 || r.prepAFFCPU > -1 {
		if r.prepAFFCPU == -1 {
			r.prepAFFCPU = 0
		}
		runtime.LockOSThread()
		if setErr := process.SetCPUAffinity(r.prepAFFCPU); setErr != nil {
			runtime.UnlockOSThread()
		} else {
			defer runtime.UnlockOSThread()
		}
	}

	ring := r.ring
	idleTime := r.prepSQEIdleTime
	if idleTime < 1 {
		idleTime = defaultPrepSQEBatchIdleTime
	}
	batchTimeWindow := r.prepSQEBatchTimeWindow
	if batchTimeWindow < 1 {
		batchTimeWindow = defaultPrepSQEBatchTimeWindow
	}

	batchTimer := time.NewTimer(batchTimeWindow)
	defer batchTimer.Stop()

	batchSize := r.prepSQEBatchSize
	if batchSize < 1 {
		batchSize = 1024
	}
	batch := make([]*Operation, batchSize)

	requestCh := r.requestCh

	var (
		batchIdx      = -1
		stopped       = false
		idle          = false
		needToPrepare = false
		needToSubmit  = 0
	)

	for {
		select {
		case <-ctx.Done():
			stopped = true
			break
		case <-batchTimer.C:
			needToPrepare = true
			break
		case op, ok := <-requestCh:
			if !ok {
				stopped = true
				break
			}
			if op == nil {
				break
			}
			if idle {
				idle = false
				batchTimer.Reset(batchTimeWindow)
			}
			batchIdx++
			batch[batchIdx] = op
			if uint32(batchIdx+1) == batchSize { // full so flush
				needToPrepare = true
			}
			break
		}
		if stopped { // check stopped
			break
		}
		if batchIdx == -1 { // when no request, use idle time
			idle = true
			batchTimer.Reset(idleTime)
			continue
		}

		if needToPrepare { // go to prepare
			needToPrepare = false
			prepared := 0
			received := batchIdx + 1
			for i := 0; i < received; i++ {
				op := batch[i]
				batchIdx--
				batch[i] = nil
				if op.canPrepare() {
					if prepErr := r.prepareSQE(op); prepErr != nil { // when prep err occur, means invalid op kind or no sqe left
						op.setResult(0, 0, prepErr)
						if errors.Is(prepErr, syscall.EBUSY) { // no sqe left
							if next := i + 1; next < received { // not last, so keep unprepared
								tmp := make([]*Operation, batchSize)
								copy(tmp, batch[next:])
								batch = tmp
							}
							break
						}
						prepared++ // prepareSQE nop whit out userdata, so prepared++
						continue
					}
					prepared++
				}
			}

			if prepared > 0 || needToSubmit > 0 { // submit
				submitted, _ := ring.Submit()
				needToSubmit += prepared - int(submitted)
				if needToSubmit < 0 {
					needToSubmit = 0
				}
			}

			// reset batch time window
			batchTimer.Reset(batchTimeWindow)
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
		sqe.SetFlags(op.sqeFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpAccept:
		addrPtr := (*syscall.RawSockaddrAny)(unsafe.Pointer(op.msg.Name))
		addrLenPtr := uint64(uintptr(unsafe.Pointer(&op.msg.Namelen)))
		if op.multishot {
			if op.directMode {
				sqe.PrepareAcceptMultishotDirect(op.fd, addrPtr, addrLenPtr, 0)
			} else {
				sqe.PrepareAcceptMultishot(op.fd, addrPtr, addrLenPtr, 0)
			}
		} else {
			if op.directMode {
				sqe.PrepareAcceptDirect(op.fd, addrPtr, addrLenPtr, 0, op.filedIndex)
			} else {
				sqe.PrepareAccept(op.fd, addrPtr, addrLenPtr, 0)
			}
		}
		sqe.SetFlags(op.sqeFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpClose:
		if op.directMode {
			sqe.PrepareCloseDirect(uint32(op.fd))
		} else {
			sqe.PrepareClose(op.fd)
		}
		sqe.SetFlags(op.sqeFlags)
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
		sqe.PrepareCancel(uintptr(op.ptr), 0)
		sqe.SetFlags(op.sqeFlags)
		break
	case iouring.OPFixedFdInstall:
		sqe.PrepareFixedFdInstall(op.fd, 0)
		sqe.SetFlags(op.sqeFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	default:
		sqe.PrepareNop()
		return UnsupportedOp
	}
	runtime.KeepAlive(sqe)
	return nil
}
