//go:build linux

package sys

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/kernel"
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

func TryGetMultipathTCPProto() (int, bool) {
	if supportsMultipathTCP() {
		return _IPPROTO_MPTCP, true
	}
	return 0, false
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
	var (
		major = 0
		minor = 0
	)
	version := kernel.Get()
	if version.Validate() {
		major, minor = version.Major, version.Minor
	}
	hasSOLMPTCP = major > 5 || (major == 5 && minor >= 16)
}

func hasFallenBack(fd *Fd) bool {
	_, err := syscall.GetsockoptInt(fd.sock, _SOL_MPTCP, _MPTCP_INFO)
	return err == syscall.EOPNOTSUPP || err == syscall.ENOPROTOOPT
}

func isUsingMPTCPProto(fd *Fd) bool {
	proto, _ := syscall.GetsockoptInt(fd.sock, syscall.SOL_SOCKET, syscall.SO_PROTOCOL)
	return proto == _IPPROTO_MPTCP
}

func IsUsingMultipathTCP(fd *Fd) bool {
	if hasSOLMPTCP {
		return !hasFallenBack(fd)
	}

	return isUsingMPTCPProto(fd)
}
