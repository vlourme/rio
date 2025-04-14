//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"sync"
	"syscall"
	"time"
)

type Listener struct {
	NetFd
	backlog          int
	acceptFn         func() (nfd *Conn, err error)
	acceptAddr       *syscall.RawSockaddrAny
	acceptAddrLenPtr *int
	operation        *Operation
	future           Future
}

func (fd *Listener) init() {
	if fd.multishot {
		fd.prepareAcceptMultishot()
		fd.acceptFn = fd.acceptMultishot
	} else {
		fd.acceptFn = fd.acceptOneshot
	}
}

func (fd *Listener) Accept() (*Conn, error) {
	return fd.acceptFn()
}

func (fd *Listener) acceptOneshot() (conn *Conn, err error) {
	acceptAddr := &syscall.RawSockaddrAny{}
	acceptAddrLen := syscall.SizeofSockaddrAny
	acceptAddrLenPtr := &acceptAddrLen

	op := AcquireOperationWithDeadline(fd.readDeadline)
	op.PrepareAccept(fd, acceptAddr, acceptAddrLenPtr)
	accepted, _, acceptErr := fd.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)

	if acceptErr != nil {
		err = acceptErr
		return
	}
	// dispatch to worker
	dispatchFd, worker, dispatchErr := fd.eventLoop.group.DispatchAndWait(accepted, fd.eventLoop)
	if dispatchErr != nil {
		err = dispatchErr
		return
	}
	// new conn
	conn = fd.newAcceptedConnFd(dispatchFd, worker)
	sa, saErr := sys.RawSockaddrAnyToSockaddr(acceptAddr)
	if saErr == nil {
		addr := sys.SockaddrToAddr(conn.net, sa)
		conn.SetRemoteAddr(addr)
	}
	return
}

func (fd *Listener) acceptMultishot() (conn *Conn, err error) {
AWAIT:
	accepted, cqeFlags, _, acceptErr := fd.future.AwaitDeadline(fd.readDeadline)
	if acceptErr != nil {
		err = acceptErr
		return
	}
	if cqeFlags&liburing.IORING_CQE_F_MORE == 0 {
		// submit
		fd.future = fd.eventLoop.Submit(fd.operation)
		goto AWAIT
	}

	// dispatch to worker
	dispatchFd, worker, dispatchErr := fd.eventLoop.group.DispatchAndWait(accepted, fd.eventLoop)
	if dispatchErr != nil {
		err = dispatchErr
		return
	}
	// new conn
	conn = fd.newAcceptedConnFd(dispatchFd, worker)
	return
}

func (fd *Listener) prepareAcceptMultishot() {
	fd.acceptAddr = &syscall.RawSockaddrAny{}
	acceptAddrLen := syscall.SizeofSockaddrAny
	fd.acceptAddrLenPtr = &acceptAddrLen

	// op
	fd.operation = AcquireOperation()
	// prepare
	fd.operation.PrepareAcceptMultishot(fd, fd.acceptAddr, fd.acceptAddrLenPtr)
	// submit
	fd.future = fd.eventLoop.Submit(fd.operation)
	return
}

func (fd *Listener) Close() error {
	if fd.operation != nil {
		_ = fd.eventLoop.Cancel(fd.operation)
		_, _, _, _ = fd.future.Await()
		op := fd.operation
		fd.operation = nil
		fd.future = nil
		ReleaseOperation(op)
	}
	return fd.NetFd.Close()
}

func (fd *Listener) newAcceptedConnFd(accepted int, event *EventLoop) (conn *Conn) {
	conn = &Conn{
		NetFd: NetFd{
			Fd: Fd{
				regular:       -1,
				direct:        accepted,
				isStream:      fd.isStream,
				zeroReadIsEOF: fd.zeroReadIsEOF,
				readDeadline:  time.Time{},
				writeDeadline: time.Time{},
				multishot:     fd.multishot,
				locker:        new(sync.Mutex),
				eventLoop:     event,
			},
			kind:             AcceptedNetFd,
			family:           fd.family,
			sotype:           fd.sotype,
			net:              fd.net,
			laddr:            nil,
			raddr:            nil,
			sendZCEnabled:    fd.sendZCEnabled,
			sendMSGZCEnabled: fd.sendZCEnabled,
		},
	}
	return
}
