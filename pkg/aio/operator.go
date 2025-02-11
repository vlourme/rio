package aio

import (
	"github.com/brickingsoft/errors"
	"net"
	"sync"
)

var (
	ErrRepeatOperation = errors.Define("the previous operation was not completed", errors.WithMeta(errMetaPkgKey, errMetaPkgVal))
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
