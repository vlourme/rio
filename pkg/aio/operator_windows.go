//go:build windows

package aio

import (
	"golang.org/x/sys/windows"
	"syscall"
	"time"
	"unsafe"
)

type Operator struct {
	overlapped syscall.Overlapped
	fd         Fd
	userdata   Userdata
	callback   OperationCallback
	completion OperatorCompletion
	timeout    time.Duration
	timer      *operatorTimer
}

type operatorCanceler struct {
	handle     syscall.Handle
	overlapped *syscall.Overlapped
}

func (op *operatorCanceler) Cancel() {
	_ = syscall.CancelIoEx(op.handle, op.overlapped)
}

func (userdata Userdata) GetMsg() *Msg {
	sm := (*windows.WSAMsg)(unsafe.Pointer(userdata.msg))
	msg := new(Msg)
	msg.FromSysMSG(sm)
	return msg
}

func (msg *Msg) FromSysMSG(sm *windows.WSAMsg) {
	// addr
	msg.Name = sm.Name
	msg.Namelen = sm.Namelen
	// buf
	if sm.BufferCount > 0 {
		buffers := unsafe.Slice(sm.Buffers, sm.BufferCount)
		for _, buf := range buffers {
			msg.AppendBuffer(unsafe.Slice(buf.Buf, buf.Len))
		}
	}
	// control
	msg.Control.Len = sm.Control.Len
	msg.Control.Buf = sm.Control.Buf
	// flags
	msg.Flags = sm.Flags
}
