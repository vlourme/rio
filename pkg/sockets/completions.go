package sockets

import "runtime"

var (
	com *completions = nil
)

type completions struct {
}

func Shutdown() {
	runtime.SetFinalizer(com, nil)
	com.shutdown()
}
