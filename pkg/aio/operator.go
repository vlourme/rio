package aio

import (
	"errors"
	"net"
	"syscall"
	"unsafe"
)

func NewBuf(b []byte) (buf Buf) {
	buf.Len = uint32(len(b))
	buf.Buf = nil
	if len(b) != 0 {
		buf.Buf = &b[0]
	}
	return
}

type Buf struct {
	Len uint32
	Buf *byte
}

func (buf *Buf) Bytes() (b []byte) {
	b = unsafe.Slice(buf.Buf, buf.Len)
	return
}

type Msg struct {
	Name        *syscall.RawSockaddrAny
	Namelen     int32
	Buffers     *Buf
	BufferCount uint32
	Control     Buf
	Flags       uint32
}

func (msg *Msg) AppendBuffer(b []byte) {
	var buffers []Buf
	if msg.Buffers != nil {
		buffers = unsafe.Slice(msg.Buffers, msg.BufferCount)
	} else {
		buffers = make([]Buf, 0, 1)
	}
	buffers = append(buffers, NewBuf(b))
	msg.Buffers = &buffers[0]
	msg.BufferCount++
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
	msg.Control.Len = uint32(len(b))
	msg.Control.Buf = nil
	if msg.Control.Len != 0 {
		msg.Control.Buf = &b[0]
	}
	return
}

func (msg *Msg) SetAddr(addr net.Addr) {
	sa := AddrToSockaddr(addr)
	name, nameLen, rawErr := SockaddrToRaw(sa)
	if rawErr != nil {
		panic(errors.New("aio.Msg: set addr failed cause invalid addr type"))
		return
	}
	msg.Name = name
	msg.Namelen = nameLen
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
	QTY     uint32
	Buf     Buf
	Msg     Msg
	Flags   uint32
	Buffers []Buf
	Handle  Fd
}

type OperationCallback func(result int, userdata Userdata, err error)

type OperatorCompletion func(result int, err error)

const (
	accept OperatorMode = iota + 1
	connect
	disconnect
	recv
	send
	recvFrom
	sendTo
	recvMsg
	sendMsg
)

const (
	acceptStr     = "accept"
	connectStr    = "connect"
	disconnectStr = "disconnect"
	recvStr       = "recv"
	sendStr       = "send"
	recvFromStr   = "recv_from"
	sendToStr     = "send_to"
	recvMsgStr    = "recv_msg"
	sendMsgStr    = "send_msg"
)

type OperatorMode uint32

func (mode OperatorMode) String() string {
	switch mode {
	case accept:
		return acceptStr
	case connect:
		return connectStr
	case disconnect:
		return disconnectStr
	case recv:
		return recvStr
	case send:
		return sendStr
	case recvFrom:
		return recvFromStr
	case sendTo:
		return sendToStr
	case recvMsg:
		return recvMsgStr
	case sendMsg:
		return sendMsgStr
	default:
		return "unknown"
	}
}
