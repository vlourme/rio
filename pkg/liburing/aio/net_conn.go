//go:build linux

package aio

import (
	"syscall"
)

type ConnFd struct {
	NetFd
	sendZCEnabled    bool
	sendMSGZCEnabled bool
	recvFn           func([]byte) (int, error)
	recvFuture       *receiveFuture
}

func (fd *ConnFd) init() {
	switch fd.sotype {
	case syscall.SOCK_STREAM: // multi recv
		if fd.vortex.MultishotReceiveEnabled() {
			future, futureErr := newReceiveFuture(fd)
			if futureErr == nil {
				fd.recvFuture = future
				fd.recvFn = fd.recvFuture.receive
			} else {
				fd.recvFn = fd.receive
			}
		} else {
			fd.recvFn = fd.receive
		}
		break
	case syscall.SOCK_DGRAM: // todo multi recv msg
		break
	default:
		break
	}
	return
}

func (fd *ConnFd) SendZCEnabled() bool {
	return fd.sendZCEnabled
}

func (fd *ConnFd) SendMSGZCEnabled() bool {
	return fd.sendMSGZCEnabled
}

func (fd *ConnFd) Close() error {
	if fd.recvFuture != nil {
		_ = fd.recvFuture.Cancel()
	}
	return fd.Fd.Close()
}

func (fd *ConnFd) CloseRead() error {
	if fd.recvFuture != nil {
		_ = fd.recvFuture.Cancel()
	}
	if fd.Registered() {
		op := fd.vortex.acquireOperation()
		op.PrepareCloseRead(fd)
		_, _, err := fd.vortex.submitAndWait(op)
		fd.vortex.releaseOperation(op)
		return err
	}
	return syscall.Shutdown(fd.regular, syscall.SHUT_RD)
}

func (fd *ConnFd) CloseWrite() error {
	if fd.Registered() {
		op := fd.vortex.acquireOperation()
		op.PrepareCloseWrite(fd)
		_, _, err := fd.vortex.submitAndWait(op)
		fd.vortex.releaseOperation(op)
		return err
	}
	return syscall.Shutdown(fd.regular, syscall.SHUT_WR)
}
