package aio

import (
	"errors"
	"io"
	"net"
)

const (
	MaxRW = 1 << 30
)

type OperatorKind int

const (
	ReadOperator = iota + 1
	WriteOperator
)

type Userdata struct {
	Fd  Fd
	QTY int
	Msg Message
}

type OperationCallback func(userdata Userdata, err error)

type OperatorCompletion func(result int, cop *Operator, err error)

func eofError(fd Fd, qty int, err error) error {
	if qty == 0 && err == nil && fd.ZeroReadIsEOF() {
		return io.EOF
	}
	return err
}

func readOperator(fd Fd) *Operator {
	op := fd.ReadOperator()
	return op
}

func writeOperator(fd Fd) *Operator {
	op := fd.WriteOperator()
	return op
}

const (
	OpDial     = "dial"
	OpListen   = "listen"
	OpAccept   = "accept"
	OpRead     = "read"
	OpWrite    = "write"
	OpClose    = "close"
	OpSet      = "set"
	OpSendfile = "sendfile"
	OpReadFrom = "readfrom"
	OpReadMsg  = "readmsg"
	OpWriteTo  = "writeto"
	OpWriteMsg = "writemsg"
)

func NewOpErr(op string, fd NetFd, err error) *net.OpError {
	var ope *net.OpError
	if ok := errors.As(err, &ope); ok && ope != nil {
		return ope
	}
	return &net.OpError{
		Op:     op,
		Net:    fd.Network(),
		Source: fd.LocalAddr(),
		Addr:   fd.RemoteAddr(),
		Err:    err,
	}
}

func NewOpWithAddrErr(op string, fd NetFd, addr net.Addr, err error) *net.OpError {
	var ope *net.OpError
	if ok := errors.As(err, &ope); ok && ope != nil {
		return ope
	}
	return &net.OpError{
		Op:     op,
		Net:    fd.Network(),
		Source: fd.LocalAddr(),
		Addr:   addr,
		Err:    err,
	}
}
