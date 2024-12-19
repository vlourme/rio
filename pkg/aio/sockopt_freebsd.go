//go:build freebsd

package aio

import (
	"golang.org/x/sys/unix"
	"os"
	"runtime"
)

func SetFastOpen(fd NetFd, n int) error {
	handle := fd.Fd()
	err := unix.SetsockoptInt(handle, unix.IPPROTO_TCP, unix.TCP_FASTOPEN, n)
	runtime.KeepAlive(fd)
	return os.NewSyscallError("setsockopt", err)
}
