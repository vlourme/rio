//go:build linux

package aio

import "time"

type Operator struct {
	fd         Fd
	userdata   Userdata
	callback   OperationCallback
	completion OperatorCompletion
	timeout    time.Duration
	timer      *operatorTimer
	cylinder   Cylinder
}

func (op *Operator) Begin() {
	op.cylinder.Up()
}

func (op *Operator) Finish() {
	op.cylinder.Down()
}
