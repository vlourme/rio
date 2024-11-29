//go:build linux

package aio

import (
	"errors"
	"golang.org/x/sys/unix"
	"sync"
	"syscall"
)

const (
	_IPPROTO_MPTCP = 0x106
	_SOL_MPTCP     = 0x11c
	_MPTCP_INFO    = 0x1
)

var (
	mptcpOnce      sync.Once
	mptcpAvailable bool
	hasSOLMPTCP    bool
)

func tryGetMultipathTCPProto() int {
	if supportsMultipathTCP() {
		return _IPPROTO_MPTCP
	}
	return 0
}

func supportsMultipathTCP() bool {
	mptcpOnce.Do(initMPTCPavailable)
	return mptcpAvailable
}

func initMPTCPavailable() {
	s, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, _IPPROTO_MPTCP)
	switch {
	case errors.Is(err, unix.EPROTONOSUPPORT):
	case errors.Is(err, unix.EINVAL):
	case err == nil:
		_ = unix.Close(s)
		fallthrough
	default:
		mptcpAvailable = true
	}
	major, minor := KernelVersion()
	hasSOLMPTCP = major > 5 || (major == 5 && minor >= 16)
}

func KernelVersion() (major, minor int) {
	var uname unix.Utsname
	if err := unix.Uname(&uname); err != nil {
		return
	}
	var (
		values    [2]int
		value, vi int
	)
	for _, c := range uname.Release {
		if '0' <= c && c <= '9' {
			value = (value * 10) + int(c-'0')
		} else {
			values[vi] = value
			vi++
			if vi >= len(values) {
				break
			}
			value = 0
		}
	}

	return values[0], values[1]
}

func hasFallenBack(fd NetFd) bool {
	_, err := syscall.GetsockoptInt(fd.Fd(), _SOL_MPTCP, _MPTCP_INFO)
	return err == syscall.EOPNOTSUPP || err == syscall.ENOPROTOOPT
}

func isUsingMPTCPProto(fd NetFd) bool {
	proto, _ := syscall.GetsockoptInt(fd.Fd(), syscall.SOL_SOCKET, syscall.SO_PROTOCOL)
	return proto == _IPPROTO_MPTCP
}

func IsUsingMultipathTCP(fd NetFd) bool {
	if hasSOLMPTCP {
		return !hasFallenBack(fd)
	}

	return isUsingMPTCPProto(fd)
}
