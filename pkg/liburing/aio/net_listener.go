//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"sync"
	"syscall"
)

type ListenerFd struct {
	*NetFd
	acceptFuture *AcceptFuture
}

func (fd *ListenerFd) Accept() (nfd *NetFd, err error) {
	if fd.acceptFuture == nil {
		direct := fd.Registered()
		deadline := fd.readDeadline
		acceptAddr := &syscall.RawSockaddrAny{}
		acceptAddrLen := syscall.SizeofSockaddrAny
		acceptAddrLenPtr := &acceptAddrLen

		op := fd.vortex.acquireOperation()
		op.WithDeadline(deadline).WithDirect(direct).PrepareAccept(fd.NetFd, acceptAddr, acceptAddrLenPtr)
		accepted, _, acceptErr := fd.vortex.submitAndWait(op)
		fd.vortex.releaseOperation(op)
		if acceptErr != nil {
			err = acceptErr
			return
		}

		nfd = fd.newAcceptedNetFd(accepted)

		sa, saErr := sys.RawSockaddrAnyToSockaddr(acceptAddr)
		if saErr == nil {
			addr := sys.SockaddrToAddr(nfd.net, sa)
			nfd.SetRemoteAddr(addr)
		}
	} else {
		nfd, _, err = fd.acceptFuture.Await()
	}
	return
}

func (fd *ListenerFd) Close() error {
	if fd.acceptFuture != nil {
		_ = fd.acceptFuture.Cancel()
	}
	return fd.NetFd.Close()
}

func (fd *ListenerFd) newAcceptedNetFd(accepted int) (cfd *NetFd) {
	cfd = &NetFd{
		Fd: Fd{
			regular:       -1,
			direct:        -1,
			isStream:      fd.isStream,
			zeroReadIsEOF: fd.zeroReadIsEOF,
			vortex:        fd.vortex,
		},
		sendZCEnabled:    fd.sendZCEnabled,
		sendMSGZCEnabled: fd.sendMSGZCEnabled,
		family:           fd.family,
		sotype:           fd.sotype,
		net:              fd.net,
		laddr:            nil,
		raddr:            nil,
	}
	if fd.Registered() {
		cfd.direct = accepted
	} else {
		cfd.regular = accepted
	}
	return
}

type AcceptFuture struct {
	ln         *ListenerFd
	op         *Operation
	addr       *syscall.RawSockaddrAny
	addrLen    *int
	buffer     int
	submitOnce sync.Once
	err        error
}

func (f *AcceptFuture) submit() error {
	f.submitOnce.Do(func() {
		direct := f.ln.Registered()
		op := NewOperation(f.buffer)
		op.Hijack()
		op.WithDirect(direct).PrepareAcceptMultishot(f.ln.NetFd, f.addr, f.addrLen)
		if ok := f.ln.vortex.Submit(op); ok {
			f.op = op
		} else {
			f.err = ErrCanceled
			op.Close()
		}
	})
	return f.err
}

func (f *AcceptFuture) Await() (fd *NetFd, cqeFlags uint32, err error) {
	if err = f.submit(); err != nil {
		return
	}
	if f.err != nil {
		err = f.err
		return
	}
	var (
		op       = f.op
		ln       = f.ln
		accepted = -1
	)
	accepted, cqeFlags, err = f.ln.vortex.awaitOperation(op) // todo handle timeout...
	f.ln.vortex.releaseOperation(op)
	if err != nil {
		return
	}
	fd = ln.newAcceptedNetFd(accepted)
	return
}

func (f *AcceptFuture) Cancel() (err error) {
	if f.op != nil {
		err = f.ln.vortex.CancelOperation(f.op)
	}
	return
}
