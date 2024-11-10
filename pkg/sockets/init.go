package sockets

import "runtime"

func init() {
	com = new(completions)
	com.poll()
	runtime.KeepAlive(com)
	runtime.SetFinalizer(com, com.shutdown)
}
