//go:build linux

package aio

import (
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

type Operator struct {
	fd         Fd
	userdata   Userdata
	callback   OperationCallback
	completion OperatorCompletion
	timeout    time.Duration
	timer      *operatorTimer
}

type operatorCanceler struct {
	cylinder *IOURingCylinder
	op       *Operator
}

func (canceler *operatorCanceler) Cancel() {
	cylinder := canceler.cylinder
	op := canceler.op
	userdata := uint64(uintptr(unsafe.Pointer(op)))
	for i := 0; i < 5; i++ {
		err := cylinder.prepare(opAsyncCancel, -1, uintptr(userdata), 0, 0, 0, userdata)
		if err == nil {
			break
		}
		if IsBusyError(err) {
			continue
		}
	}
	runtime.KeepAlive(userdata)
}

func (userdata Userdata) GetMsg() *Msg {
	sm := (*syscall.Msghdr)(unsafe.Pointer(userdata.msg))
	msg := new(Msg)
	msg.FromSysMSG(sm)
	return msg
}

func (msg *Msg) FromSysMSG(sm *syscall.Msghdr) {
	// addr
	msg.Name = (*syscall.RawSockaddrAny)(unsafe.Pointer(sm.Name))
	msg.Namelen = int32(sm.Namelen)
	// buf
	if sm.Iovlen > 0 {
		iovs := unsafe.Slice(sm.Iov, sm.Iovlen)
		for _, iov := range iovs {
			msg.AppendBuffer(unsafe.Slice(iov.Base, iov.Len))
		}
	}
	// control
	msg.Control.Len = uint32(sm.Controllen)
	msg.Control.Buf = sm.Control
	// flags
	msg.Flags = uint32(sm.Flags)
}

func (msg *Msg) ToSysMSG() (sm *syscall.Msghdr) {
	sm = new(syscall.Msghdr)
	// addr
	sm.Name = (*byte)(unsafe.Pointer(msg.Name))
	sm.Namelen = uint32(msg.Namelen)
	// buf
	var ioves []syscall.Iovec
	if msg.BufferCount > 0 {
		ioves = make([]syscall.Iovec, msg.BufferCount)
		for i := 0; i < int(msg.BufferCount); i++ {
			buf, _ := msg.Buf(i)
			ioves[i] = syscall.Iovec{
				Base: buf.Buf,
				Len:  uint64(buf.Len),
			}
		}
		sm.Iov = (*syscall.Iovec)(unsafe.Pointer(&ioves[0]))
		sm.Iovlen = uint64(msg.BufferCount)
	}
	// control
	if msg.Control.Len > 0 {
		sm.Control = msg.Control.Buf
		sm.Controllen = uint64(msg.Control.Len)
	}
	// flags
	sm.Flags = int32(msg.Flags)
	return
}
