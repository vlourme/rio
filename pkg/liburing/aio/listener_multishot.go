//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

func newMultishotAcceptor(ln *Listener) (acceptor *MultishotAcceptor) {
	acceptor = &MultishotAcceptor{
		serving:         true,
		acceptAddr:      &syscall.RawSockaddrAny{},
		acceptAddrLen:   syscall.SizeofSockaddrAny,
		eventLoop:       ln.eventLoop,
		operation:       &Operation{},
		future:          nil,
		err:             nil,
		locker:          new(sync.Mutex),
		operationLocker: new(sync.Mutex),
	}
	acceptor.operation.PrepareAcceptMultishot(ln, acceptor.acceptAddr, &acceptor.acceptAddrLen)
	acceptor.future = acceptor.eventLoop.Submit(acceptor.operation)
	return
}

type MultishotAcceptor struct {
	serving         bool
	acceptAddr      *syscall.RawSockaddrAny
	acceptAddrLen   int
	eventLoop       *EventLoop
	operation       *Operation
	future          Future
	err             error
	locker          *sync.Mutex
	operationLocker *sync.Mutex
}

func (acceptor *MultishotAcceptor) Handle(n int, flags uint32, err error) (bool, int, uint32, unsafe.Pointer, error) {
	return true, n, flags, nil, err
}

func (acceptor *MultishotAcceptor) Accept(deadline time.Time) (fd int, eventLoop *EventLoop, err error) {
	acceptor.locker.Lock()
	if acceptor.err != nil {
		err = acceptor.err

		acceptor.locker.Unlock()
		return
	}

	var (
		accepted int
		flags    uint32
	)
	accepted, flags, _, err = acceptor.future.AwaitDeadline(deadline)
	if err != nil {
		acceptor.serving = false
		acceptor.err = err
		if IsTimeout(err) {
			if acceptor.cancel() {
				for {
					_, _, _, waitErr := acceptor.future.Await()
					if IsCanceled(waitErr) {
						break
					}
				}
			}
		}
		acceptor.locker.Unlock()
		return
	}

	if flags&liburing.IORING_CQE_F_MORE == 0 {
		acceptor.locker.Lock()
		acceptor.serving = false
		acceptor.err = ErrCanceled
		err = acceptor.err

		acceptor.locker.Unlock()
		return
	}
	// dispatch
	fd, eventLoop, err = acceptor.eventLoop.Group().Dispatch(accepted, acceptor.eventLoop)

	acceptor.locker.Unlock()
	return
}

func (acceptor *MultishotAcceptor) cancel() bool {
	acceptor.operationLocker.Lock()
	defer acceptor.operationLocker.Unlock()
	if op := acceptor.operation; op != nil {
		return acceptor.eventLoop.Cancel(acceptor.operation) == nil
	}
	return false
}

func (acceptor *MultishotAcceptor) Close() (err error) {
	if acceptor.locker.TryLock() {
		if acceptor.serving {
			acceptor.serving = false
			if acceptor.cancel() {
				for {
					_, _, _, waitErr := acceptor.future.Await()
					if IsCanceled(waitErr) {
						break
					}
				}
			}
		}
		acceptor.err = ErrCanceled
		acceptor.locker.Unlock()
		return
	}
	acceptor.cancel()
	return
}
