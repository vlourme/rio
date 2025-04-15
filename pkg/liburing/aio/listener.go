//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

func newMultishotAcceptor(ln *Listener) (acceptor *MultishotAcceptor) {
	acceptor = &MultishotAcceptor{
		serving:       true,
		acceptAddr:    &syscall.RawSockaddrAny{},
		acceptAddrLen: syscall.SizeofSockaddrAny,
		eventLoop:     ln.eventLoop,
		operation:     &Operation{},
		future:        nil,
		err:           nil,
		locker:        new(sync.Mutex),
	}
	acceptor.operation.PrepareAcceptMultishot(ln, acceptor.acceptAddr, &acceptor.acceptAddrLen)
	acceptor.future = acceptor.eventLoop.Submit(acceptor.operation)
	return
}

type MultishotAcceptor struct {
	serving       bool
	acceptAddr    *syscall.RawSockaddrAny
	acceptAddrLen int
	eventLoop     *EventLoop
	operation     *Operation
	future        Future
	err           error
	locker        sync.Locker
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
		acceptor.locker.Unlock()
		return
	}
	if flags&liburing.IORING_CQE_F_MORE == 0 {
		acceptor.serving = false
		acceptor.err = ErrCanceled
		err = acceptor.err
		acceptor.locker.Unlock()
		return
	}
	// dispatch
	fd, eventLoop, err = acceptor.eventLoop.group.Dispatch(accepted, acceptor.eventLoop)
	acceptor.locker.Unlock()
	return
}

func (acceptor *MultishotAcceptor) Close() (err error) {
	err = acceptor.eventLoop.Cancel(acceptor.operation)
	return
}

type Listener struct {
	NetFd
	multishotAcceptOnce sync.Once
	multishotAcceptor   *MultishotAcceptor
}

func (fd *Listener) Accept() (*Conn, error) {
	if fd.multishot {
		fd.multishotAcceptOnce.Do(func() {
			fd.multishotAcceptor = newMultishotAcceptor(fd)
		})
		var (
			accepted int
			member   *EventLoop
			err      error
		)
		accepted, member, err = fd.multishotAcceptor.Accept(fd.readDeadline)
		if err != nil {
			return nil, err
		}
		// new conn
		conn := fd.newAcceptedConnFd(accepted, member)
		return conn, nil
	}

	return fd.acceptOneshot()
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
	// dispatch to member
	dispatchFd, member, dispatchErr := fd.eventLoop.group.Dispatch(accepted, fd.eventLoop)
	if dispatchErr != nil {
		err = dispatchErr
		return
	}
	// new conn
	conn = fd.newAcceptedConnFd(dispatchFd, member)
	sa, saErr := sys.RawSockaddrAnyToSockaddr(acceptAddr)
	if saErr == nil {
		addr := sys.SockaddrToAddr(conn.net, sa)
		conn.SetRemoteAddr(addr)
	}
	return
}

func (fd *Listener) Close() error {
	if fd.multishot {
		if err := fd.multishotAcceptor.Close(); err != nil {
			fd.Cancel()
			err = nil
		}
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
