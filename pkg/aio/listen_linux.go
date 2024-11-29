//go:build linux

package aio

import (
	"bufio"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func newListenerFd(network string, family int, sotype int, proto int, addr net.Addr, multicastInterface *net.Interface) (v *netFd, err error) {

	// syscall.Listen(0, somaxconn)

	return
}

func Accept(fd NetFd, cb OperationCallback) {

}

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

func SetNoDelay(fd NetFd, noDelay bool) (err error) {
	handle := fd.Fd()
	err = syscall.SetsockoptInt(handle, syscall.IPPROTO_TCP, syscall.TCP_NODELAY, boolint(noDelay))
	if err != nil {
		err = os.NewSyscallError("setsockopt", err)
		return
	}
	return
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

func SetKeepAlivePeriod(fd NetFd, period time.Duration) (err error) {
	if period == 0 {
		period = defaultTCPKeepAliveIdle
	} else if period < 0 {
		return nil
	}
	secs := int(roundDurationUp(period, time.Second))
	handle := fd.Fd()
	err = syscall.SetsockoptInt(handle, syscall.IPPROTO_TCP, syscall.TCP_KEEPIDLE, secs)
	if err != nil {
		err = os.NewSyscallError("setsockopt", err)
		return
	}
	return
}

var (
	somaxconn = maxListenerBacklog()
)

const (
	maxUint16Value = 1<<16 - 1
)

func maxListenerBacklog() int {
	fd, err := os.Open("/proc/sys/net/core/somaxconn")
	if err != nil {
		return syscall.SOMAXCONN
	}
	defer fd.Close()

	rd := bufio.NewReader(fd)

	line, err := rd.ReadString('\n')
	if err != nil {
		return syscall.SOMAXCONN
	}

	f := strings.Fields(line)
	if len(f) < 1 {
		return syscall.SOMAXCONN
	}

	value, err := strconv.Atoi(f[0])
	if err != nil || value == 0 {
		return syscall.SOMAXCONN
	}

	// Linux stores the backlog in a uint16.
	// Truncate number to avoid wrapping.
	// See issue 5030.
	if value > maxUint16Value {
		value = maxUint16Value
	}

	return value
}
