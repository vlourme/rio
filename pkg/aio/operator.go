package aio

import (
	"io"
	"net"
)

const (
	MaxRW = 1 << 30
)

type Message interface {
	Addr() (addr net.Addr, err error)
	Bytes(n int) (b []byte)
	ControlBytes() (b []byte)
	ControlLen() (n int)
	Flags() int32
}

type Userdata struct {
	Fd  Fd
	QTY uint32
	Msg Message
}

type OperationCallback func(result int, userdata Userdata, err error)

type OperatorCompletion func(result int, op *Operator, err error)

func eofError(fd Fd, qty int, err error) error {
	if qty == 0 && err == nil && fd.ZeroReadIsEOF() {
		return io.EOF
	}
	return err
}
