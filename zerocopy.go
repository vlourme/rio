package rio

import "sync/atomic"

var (
	defaultUseSendZC = atomic.Bool{}
)

func UseZeroCopy(use bool) {
	defaultUseSendZC.Store(use)
}
