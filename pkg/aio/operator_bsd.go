//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"net"
	"syscall"
	"time"
)

type Operator struct {
	userdata Userdata
	timeout  time.Duration
}

type Message struct{}

func (msg *Message) Addr() (addr net.Addr, err error) {

	return
}

func (msg *Message) Bytes(n int) (b []byte) {
	return
}

func (msg *Message) ControlBytes() (b []byte) {
	return
}

func (msg *Message) ControlLen() int {
	return 0
}

func (msg *Message) Flags() int32 {
	return 0
}

func (msg *Message) BuildRawSockaddrAny() (*syscall.RawSockaddrAny, int32) {

	return nil, 0
}

func (msg *Message) SetAddr(addr net.Addr) (sa syscall.Sockaddr, err error) {

	return
}

func (msg *Message) Append(b []byte) (buf syscall.Iovec) {

	return
}

func (msg *Message) SetControl(b []byte) {
	return
}

func (msg *Message) SetFlags(flags uint32) {

}
