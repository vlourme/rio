package aio

import (
	"errors"
	"net"
	"sync"
)

type Userdata struct {
	Fd           Fd
	N            int
	OOBN         int
	Addr         net.Addr
	MessageFlags int
}

type OperationCallback func(userdata Userdata, err error)

type OperatorCompletion func(result int, cop *Operator, err error)

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

var (
	operators = sync.Pool{New: func() interface{} {
		return &Operator{}
	}}
)

func acquireOperator() *Operator {
	return operators.Get().(*Operator)
}

func releaseOperator(op *Operator) {
	if op.reset() {
		operators.Put(op)
	}
}
