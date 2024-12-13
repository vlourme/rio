//go:build linux

package aio

import (
	"errors"
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
	s, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM|syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC, _IPPROTO_MPTCP)
	switch {
	case errors.Is(err, syscall.EPROTONOSUPPORT):
	case errors.Is(err, syscall.EINVAL):
	case err == nil:
		_ = syscall.Close(s)
		fallthrough
	default:
		mptcpAvailable = true
	}
	major, minor := KernelVersion()
	hasSOLMPTCP = major > 5 || (major == 5 && minor >= 16)
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
