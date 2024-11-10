package sockets

import "runtime"

func init() {
	com = new(completions)
	com.poll()
	runtime.SetFinalizer(com, com.shutdown)
}
