//go:build openbsd

package aio

import (
	"os"
	"runtime"
	"syscall"
	"time"
)

func SetReadBuffer(fd NetFd, n int) (err error) {
	handle := fd.Fd()
	err = syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, syscall.SO_RCVBUF, n)
	if err != nil {
		err = os.NewSyscallError("setsockopt", err)
		return
	}
	return
}

func SetWriteBuffer(fd NetFd, n int) (err error) {
	handle := fd.Fd()
	err = syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, syscall.SO_SNDBUF, n)
	if err != nil {
		err = os.NewSyscallError("setsockopt", err)
		return
	}
	return
}

func SetNoDelay(fd NetFd, noDelay bool) error {
	handle := fd.Fd()
	err := syscall.SetsockoptInt(handle, syscall.IPPROTO_TCP, syscall.TCP_NODELAY, boolint(noDelay))
	runtime.KeepAlive(fd)
	return os.NewSyscallError("setsockopt", err)
}

func SetLinger(fd NetFd, sec int) (err error) {
	handle := fd.Fd()
	var l syscall.Linger
	if sec >= 0 {
		l.Onoff = 1
		l.Linger = int32(sec)
	} else {
		l.Onoff = 0
		l.Linger = 0
	}
	err = syscall.SetsockoptLinger(handle, syscall.SOL_SOCKET, syscall.SO_LINGER, &l)
	if err != nil {
		err = os.NewSyscallError("setsockopt", err)
		return
	}
	return
}

func SetKeepAlive(fd NetFd, keepalive bool) (err error) {
	handle := fd.Fd()
	err = syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, boolint(keepalive))
	if err != nil {
		err = os.NewSyscallError("setsockopt", err)
		return
	}
	return
}

func SetKeepAlivePeriod(_ NetFd, _ time.Duration) error {
	return nil
}

func setKeepAliveInterval(_ NetFd, _ time.Duration) error {
	return nil
}

func setKeepAliveCount(_ NetFd, _ int) error {
	return nil
}

func SetKeepAliveConfig(fd NetFd, config KeepAliveConfig) error {
	if err := SetKeepAlive(fd, config.Enable); err != nil {
		return err
	}
	return nil
}
