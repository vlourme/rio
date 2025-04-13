//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	_supportListen    bool
	_supportBind      bool
	_supportSendZC    bool
	_supportSendMSGZC bool
	_supportOnce      sync.Once
	_msgs             sync.Pool
	_timers           sync.Pool
)

func supportOpCheck() {
	_supportOnce.Do(func() {
		probe, probeErr := liburing.GetProbe()
		if probeErr != nil {
			return
		}
		_supportListen = probe.IsSupported(liburing.IORING_OP_LISTEN)
		_supportBind = probe.IsSupported(liburing.IORING_OP_BIND)
		_supportSendZC = probe.IsSupported(liburing.IORING_OP_SEND_ZC)
		_supportSendMSGZC = probe.IsSupported(liburing.IORING_OP_SENDMSG_ZC)
	})
}

func supportListen() bool {
	supportOpCheck()
	return _supportListen
}

func supportBind() bool {
	supportOpCheck()
	return _supportBind
}

func supportSendZC() bool {
	supportOpCheck()
	return _supportSendZC
}

func supportSendMSGZC() bool {
	supportOpCheck()
	return _supportSendMSGZC
}

func acquireTimer(timeout time.Duration) *time.Timer {
	v := _timers.Get()
	if v == nil {
		timer := time.NewTimer(timeout)
		return timer
	}
	timer := v.(*time.Timer)
	timer.Reset(timeout)
	return timer
}

func releaseTimer(timer *time.Timer) {
	timer.Stop()
	_timers.Put(timer)
}

func acquireMsg(b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32) *syscall.Msghdr {
	var msg *syscall.Msghdr
	msg0 := _msgs.Get()
	if msg0 == nil {
		msg = &syscall.Msghdr{}
	} else {
		msg = msg0.(*syscall.Msghdr)
	}
	bLen := len(b)
	if bLen > 0 {
		msg.Iov = &syscall.Iovec{
			Base: &b[0],
			Len:  uint64(bLen),
		}
		msg.Iovlen = 1
	}
	oobLen := len(oob)
	if oobLen > 0 {
		msg.Control = &oob[0]
		msg.SetControllen(oobLen)
	}
	if addr != nil {
		msg.Name = (*byte)(unsafe.Pointer(addr))
		msg.Namelen = uint32(addrLen)
	}
	msg.Flags = flags
	return msg
}

func releaseMsg(msg *syscall.Msghdr) {
	msg.Name = nil
	msg.Namelen = 0
	msg.Iov = nil
	msg.Iovlen = 0
	msg.Control = nil
	msg.Controllen = 0
	msg.Flags = 0
	_msgs.Put(msg)
	return
}
