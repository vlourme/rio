package iouring

import (
	"runtime"
	"syscall"
	"unsafe"
)

const (
	SetupIOPoll uint32 = 1 << iota
	SetupSQPoll
	SetupSQAff
	SetupCQSize
	SetupClamp
	SetupAttachWQ
	SetupRDisabled
	SetupSubmitAll
	SetupCoopTaskrun
	SetupTaskrunFlag
	SetupSQE128
	SetupCQE32
	SetupSingleIssuer
	SetupDeferTaskrun
	SetupNoMmap
	SetupRegisteredFdOnly
)

// liburing: io_uring_params
type Params struct {
	sqEntries    uint32
	cqEntries    uint32
	flags        uint32
	sqThreadCPU  uint32
	sqThreadIdle uint32
	features     uint32
	wqFd         uint32
	resv         [3]uint32

	sqOff SQRingOffsets
	cqOff CQRingOffsets
}

// liburing: io_uring_setup - https://manpages.debian.org/unstable/liburing-dev/io_uring_setup.2.en.html
func Setup(entries uint32, p *Params) (uint, error) {
	fd, _, errno := syscall.Syscall(sysSetup, uintptr(entries), uintptr(unsafe.Pointer(p)), 0)
	runtime.KeepAlive(p)

	return uint(fd), errno
}
