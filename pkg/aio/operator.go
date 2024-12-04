package aio

import (
	"errors"
	"io"
	"net"
	"syscall"
	"unsafe"
)

const (
	MaxRW = 1 << 30
)

func NewBuf(b []byte) (buf Buf) {
	buf.Len = uint64(len(b))
	buf.Buf = nil
	if len(b) != 0 {
		buf.Buf = &b[0]
	}
	return
}

type Buf struct {
	Buf *byte
	Len uint64
}

func (buf *Buf) Bytes() (b []byte) {
	b = unsafe.Slice(buf.Buf, buf.Len)
	return
}

type Msg struct {
	Name        *syscall.RawSockaddrAny
	NameLen     uint32
	Pad_cgo_0   [4]byte
	Buffers     *Buf
	BufferCount uint64
	Control     *byte
	ControlLen  uint64
	Flags       int32
	Pad_cgo_1   [4]byte
}

func (msg *Msg) AppendBuffer(b []byte) (buf Buf) {
	var buffers []Buf
	if msg.Buffers != nil {
		buffers = unsafe.Slice(msg.Buffers, msg.BufferCount)
	} else {
		buffers = make([]Buf, 0, 1)
	}
	buf = NewBuf(b)
	buffers = append(buffers, buf)
	msg.Buffers = &buffers[0]
	msg.BufferCount++
	return
}

func (msg *Msg) Buf(index int) (buf Buf, err error) {
	if index >= int(msg.BufferCount) {
		err = errors.New("aio.Msg: get buf failed, index out of range")
		return
	}
	buffers := unsafe.Slice(msg.Buffers, msg.BufferCount)
	buf = buffers[index]
	return
}

func (msg *Msg) SetControl(b []byte) {
	msg.ControlLen = uint64(len(b))
	msg.Control = nil
	if msg.ControlLen != 0 {
		msg.Control = &b[0]
	}
	return
}

func (msg *Msg) ControlBytes() []byte {
	if msg.Control != nil {
		return unsafe.Slice(msg.Control, int(msg.ControlLen))
	}
	return nil
}

func (msg *Msg) SetAddr(addr net.Addr) {
	sa := AddrToSockaddr(addr)
	name, nameLen, rawErr := SockaddrToRaw(sa)
	if rawErr != nil {
		panic(errors.New("aio.Msg: set addr failed cause invalid addr type"))
		return
	}
	msg.Name = name
	msg.NameLen = uint32(nameLen)
	return
}

func (msg *Msg) Addr() (addr net.Addr, err error) {
	if msg.Name == nil {
		return
	}
	sa, saErr := RawToSockaddr(msg.Name)
	if saErr != nil {
		err = errors.Join(errors.New("aio.Msg: get addr failed"), saErr)
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
				err = errors.Join(errors.New("aio.Msg: get addr failed"), ifiErr)
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
		err = errors.Join(errors.New("aio.Msg: get addr failed"), errors.New("unknown address type"))
		return
	}
	return
}

type Userdata struct {
	Fd  Fd
	QTY uint32
	msg uintptr
}

type OperationCallback func(result int, userdata Userdata, err error)

type OperatorCompletion func(result int, op *Operator, err error)

func eofError(fd Fd, qty int, err error) error {
	if qty == 0 && err == nil && fd.ZeroReadIsEOF() {
		return io.EOF
	}
	return err
}
