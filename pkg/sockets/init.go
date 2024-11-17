package sockets

import "runtime"

func init() {
	com = new(completions)
	runtime.SetFinalizer(com, (*completions).shutdown)
	com.poll()
	runtime.KeepAlive(com)
}
