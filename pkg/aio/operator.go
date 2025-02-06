package aio

import (
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

var (
	operators = sync.Pool{New: func() interface{} {
		return &Operator{}
	}}
)

func acquireOperator(fd Fd) *Operator {
	op := operators.Get().(*Operator)
	op.setFd(fd)
	return op
}

func releaseOperator(op *Operator) {
	op.reset()
	operators.Put(op)
}
