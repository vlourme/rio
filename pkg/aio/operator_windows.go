//go:build windows

package aio

import (
	"errors"
	"golang.org/x/sys/windows"
	"net"
	"syscall"
	"time"
	"unsafe"
)

func newOperator(fd Fd) *Operator {
	return &Operator{
		overlapped: syscall.Overlapped{},
		fd:         fd,
		handle:     -1,
		n:          0,
		msg:        Message{},
		callback:   nil,
		completion: nil,
	}
}

type Operator struct {
	overlapped syscall.Overlapped
	fd         Fd
	handle     int
	n          uint32
	msg        Message
	callback   OperationCallback
	completion OperatorCompletion
	timeout    time.Duration
	timer      *operatorTimer
}

func (op *Operator) tryPrepareTimeout() {
	if op.timeout > 0 {
		op.timer = getOperatorTimer()
		op.timer.Start(op.timeout, &operatorCanceler{
			op: op,
		})
	}
}

func (op *Operator) deadlineExceeded() (ok bool) {
	if timer := op.timer; timer != nil {
		ok = timer.DeadlineExceeded()
	}
	return
}

func (op *Operator) tryResetTimeout() {
	if timer := op.timer; timer != nil {
		timer.Done()
		putOperatorTimer(timer)
		op.timer = nil
	}
}

func (op *Operator) clean() {
	op.overlapped = syscall.Overlapped{}
	op.handle = -1
	op.n = 0
	op.msg = Message{}
	op.callback = nil
	op.completion = nil
	op.tryResetTimeout()
}

type operatorCanceler struct {
	op *Operator
}

func (canceler *operatorCanceler) Cancel() {
	if op := canceler.op; op != nil {
		if fd := op.fd; fd != nil {
			handle := syscall.Handle(fd.Fd())
			overlapped := &op.overlapped
			_ = syscall.CancelIoEx(handle, overlapped)

		}
	}
}

type Message struct {
	windows.WSAMsg
}

func (msg *Message) Addr() (addr net.Addr, err error) {
	if msg.Name == nil {
		err = errors.Join(errors.New("aio.Message: get addr failed"), errors.New("addr is nil"))
		return
	}
	sa, saErr := RawToSockaddr(msg.Name)
	if saErr != nil {
		err = errors.Join(errors.New("aio.Message: get addr failed"), saErr)
		return
	}

	switch a := sa.(type) {
	case *syscall.SockaddrInet4:
		addr = &net.UDPAddr{
			IP:   append([]byte{}, a.Addr[:]...),
			Port: a.Port,
		}
		break
	case *syscall.SockaddrInet6:
		zone := ""
		if a.ZoneId != 0 {
			ifi, ifiErr := net.InterfaceByIndex(int(a.ZoneId))
			if ifiErr != nil {
				err = errors.Join(errors.New("aio.Message: get addr failed"), ifiErr)
			}
			zone = ifi.Name
		}
		addr = &net.UDPAddr{
			IP:   append([]byte{}, a.Addr[:]...),
			Port: a.Port,
			Zone: zone,
		}
		break
	case *syscall.SockaddrUnix:
		addr = &net.UnixAddr{Net: "unixgram", Name: a.Name}
		break
	default:
		err = errors.Join(errors.New("aio.Message: get addr failed"), errors.New("unknown address type"))
		return
	}
	return
}

func (msg *Message) Bytes(n int) (b []byte) {
	if n < 0 || n > int(msg.BufferCount) {
		return
	}
	if msg.BufferCount == 0 {
		return
	}
	buffers := unsafe.Slice(msg.Buffers, msg.BufferCount)
	buffer := buffers[n]
	b = unsafe.Slice(buffer.Buf, buffer.Len)
	return
}

func (msg *Message) ControlBytes() (b []byte) {
	if msg.Control.Len == 0 {
		return
	}
	b = unsafe.Slice(msg.Control.Buf, msg.Control.Len)
	return
}

func (msg *Message) ControlLen() int {
	return int(msg.Control.Len)
}

func (msg *Message) Flags() int32 {
	return int32(msg.WSAMsg.Flags)
}

func (msg *Message) BuildRawSockaddrAny() (*syscall.RawSockaddrAny, int32) {
	msg.WSAMsg.Name = new(syscall.RawSockaddrAny)
	msg.WSAMsg.Namelen = int32(unsafe.Sizeof(*msg.WSAMsg.Name))
	return msg.WSAMsg.Name, msg.WSAMsg.Namelen
}

func (msg *Message) SetAddr(addr net.Addr) (sa syscall.Sockaddr, err error) {
	sa = AddrToSockaddr(addr)
	name, nameLen, rawErr := SockaddrToRaw(sa)
	if rawErr != nil {
		panic(errors.New("aio.Message: set addr failed cause invalid addr type"))
		return
	}
	msg.Name = name
	msg.Namelen = nameLen
	return
}

func (msg *Message) Append(b []byte) (buf syscall.WSABuf) {
	buf = syscall.WSABuf{
		Len: uint32(len(b)),
		Buf: nil,
	}
	if buf.Len > 0 {
		buf.Buf = &b[0]
	}
	wsabuf := (*windows.WSABuf)(unsafe.Pointer(&buf))
	if msg.BufferCount == 0 {
		msg.Buffers = wsabuf
	} else {
		buffers := unsafe.Slice(msg.Buffers, msg.BufferCount)
		buffers = append(buffers, *wsabuf)
		msg.Buffers = (*windows.WSABuf)(unsafe.Pointer(&buffers[0]))
	}
	msg.BufferCount++
	return
}

func (msg *Message) SetControl(b []byte) {
	msg.Control.Len = uint32(len(b))
	if msg.Control.Len > 0 {
		msg.Control.Buf = &b[0]
	}
}

func (msg *Message) SetFlags(flags uint32) {
	msg.WSAMsg.Flags = flags
}
