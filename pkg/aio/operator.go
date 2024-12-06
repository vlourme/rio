package aio

import (
	"io"
)

const (
	MaxRW = 1 << 30
)

type Userdata struct {
	Fd  Fd
	QTY uint32
	Msg Message
}

type OperationCallback func(result int, userdata Userdata, err error)

type OperatorCompletion func(result int, cop *Operator, err error)

func eofError(fd Fd, qty int, err error) error {
	if qty == 0 && err == nil && fd.ZeroReadIsEOF() {
		return io.EOF
	}
	return err
}

func ReadOperator(fd Fd) *Operator {
	op := fd.ReadOperator()
	return &op
}

func WriteOperator(fd Fd) *Operator {
	op := fd.WriteOperator()
	return &op
}
