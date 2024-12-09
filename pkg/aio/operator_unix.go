//go:build unix

package aio

import (
	"time"
)

type Operator struct {
	userdata   Userdata
	fd         Fd
	callback   OperationCallback
	completion OperatorCompletion
	timeout    time.Duration
	timer      *operatorTimer
}
