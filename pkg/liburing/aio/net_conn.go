//go:build linux

package aio

import (
	"sync"
	"syscall"
)

type ConnFd struct {
	NetFd
	sendZCEnabled    bool
	sendMSGZCEnabled bool
	recvFuture       *receiveFuture
}

func (fd *ConnFd) init() {
	switch fd.sotype {
	case syscall.SOCK_STREAM: // multi recv
		if fd.vortex.MultishotReceiveEnabled() {
			buffer := fd.vortex.bufferConfig.AcquireBuffer()
			if buffer == nil {
				break
			}
			fd.recvFuture = &receiveFuture{
				fd:         fd,
				op:         nil,
				buffer:     buffer,
				submitOnce: sync.Once{},
				err:        nil,
			}
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
