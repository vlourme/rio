//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"sync"
	"syscall"
	"time"
)

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
	dispatchFd, member, dispatchErr := fd.eventLoop.Group().Dispatch(accepted, fd.eventLoop)
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
			err = nil
		}
	}
	return fd.NetFd.Close()
}

func (fd *Listener) newAcceptedConnFd(accepted int, event *EventLoop) (conn *Conn) {
	conn = &Conn{
		NetFd: NetFd{
			Fd: Fd{
				locker:        sync.Mutex{},
				regular:       -1,
				direct:        accepted,
				isStream:      fd.isStream,
				zeroReadIsEOF: fd.zeroReadIsEOF,
				readDeadline:  time.Time{},
				writeDeadline: time.Time{},
				multishot:     fd.multishot,
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
