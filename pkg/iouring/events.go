package iouring

// liburing: io_uring_getevents_arg
type GetEventsArg struct {
	sigMask   uint64
	sigMaskSz uint32
	pad       uint32
	ts        uint64
}
