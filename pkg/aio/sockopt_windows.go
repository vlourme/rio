//go:build windows

package aio

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

func SetReadBuffer(fd NetFd, n int) (err error) {
	handle := windows.Handle(fd.Fd())
	err = windows.SetsockoptInt(handle, windows.SOL_SOCKET, windows.SO_RCVBUF, n)
	if err != nil {
		err = os.NewSyscallError("setsockopt", err)
		return
	}
	return
}

func SetWriteBuffer(fd NetFd, n int) (err error) {
	handle := windows.Handle(fd.Fd())
	err = windows.SetsockoptInt(handle, windows.SOL_SOCKET, windows.SO_SNDBUF, n)
	if err != nil {
		err = os.NewSyscallError("setsockopt", err)
		return
	}
	return
}

func SetNoDelay(fd NetFd, noDelay bool) error {
	handle := windows.Handle(fd.Fd())
	err := windows.SetsockoptInt(handle, windows.IPPROTO_TCP, windows.TCP_NODELAY, boolint(noDelay))
	runtime.KeepAlive(fd)
	return os.NewSyscallError("setsockopt", err)
}

func SetLinger(fd NetFd, sec int) (err error) {
	handle := windows.Handle(fd.Fd())
	var l windows.Linger
	if sec >= 0 {
		l.Onoff = 1
		l.Linger = int32(sec)
	} else {
		l.Onoff = 0
		l.Linger = 0
	}
	err = windows.SetsockoptLinger(handle, windows.SOL_SOCKET, windows.SO_LINGER, &l)
	if err != nil {
		err = os.NewSyscallError("setsockopt", err)
		return
	}
	return
}

func SetKeepAlive(fd NetFd, keepalive bool) (err error) {
	handle := windows.Handle(fd.Fd())
	err = windows.SetsockoptInt(handle, windows.SOL_SOCKET, windows.SO_KEEPALIVE, boolint(keepalive))
	if err != nil {
		err = os.NewSyscallError("setsockopt", err)
		return
	}
	return
}

func SetKeepAlivePeriod(fd NetFd, d time.Duration) error {
	if !SupportTCPKeepAliveIdle() {
		return setKeepAliveIdleAndInterval(fd, d, -1)
	}

	if d == 0 {
		d = defaultTCPKeepAliveIdle
	} else if d < 0 {
		return nil
	}
	secs := int(roundDurationUp(d, time.Second))
	handle := windows.Handle(fd.Fd())
	err := windows.SetsockoptInt(handle, windows.IPPROTO_TCP, windows.TCP_KEEPIDLE, secs)
	runtime.KeepAlive(fd)
	return os.NewSyscallError("setsockopt", err)
}

func SetKeepAliveConfig(fd NetFd, config KeepAliveConfig) error {
	if err := SetKeepAlive(fd, config.Enable); err != nil {
		return err
	}
	if SupportTCPKeepAliveIdle() && SupportTCPKeepAliveInterval() {
		if err := SetKeepAlivePeriod(fd, config.Idle); err != nil {
			return err
		}
		if err := setKeepAliveInterval(fd, config.Interval); err != nil {
			return err
		}
	} else if err := setKeepAliveIdleAndInterval(fd, config.Idle, config.Interval); err != nil {
		return err
	}
	if err := setKeepAliveCount(fd, config.Count); err != nil {
		return err
	}
	return nil
}

func setKeepAliveInterval(fd NetFd, d time.Duration) error {
	if !SupportTCPKeepAliveInterval() {
		return setKeepAliveIdleAndInterval(fd, -1, d)
	}

	if d == 0 {
		d = defaultTCPKeepAliveInterval
	} else if d < 0 {
		return nil
	}
	secs := int(roundDurationUp(d, time.Second))
	handle := windows.Handle(fd.Fd())
	err := windows.SetsockoptInt(handle, windows.IPPROTO_TCP, windows.TCP_KEEPINTVL, secs)
	runtime.KeepAlive(fd)
	return os.NewSyscallError("setsockopt", err)
}

func setKeepAliveCount(fd NetFd, n int) error {
	if n == 0 {
		n = defaultTCPKeepAliveCount
	} else if n < 0 {
		return nil
	}
	handle := windows.Handle(fd.Fd())
	err := windows.SetsockoptInt(handle, windows.IPPROTO_TCP, windows.TCP_KEEPCNT, n)
	runtime.KeepAlive(fd)
	return os.NewSyscallError("setsockopt", err)
}

const (
	defaultKeepAliveInterval = time.Second
)

func setKeepAliveIdleAndInterval(fd NetFd, idle, interval time.Duration) error {
	switch {
	case idle < 0 && interval >= 0:
		// Given that we can't set KeepAliveInterval alone, and this code path
		// is new, it doesn't exist before, so we just return an error.
		return windows.WSAENOPROTOOPT
	case idle >= 0 && interval < 0:
		// Although we can't set KeepAliveTime alone either, this existing code
		// path had been backing up [SetKeepAlivePeriod] which used to be set both
		// KeepAliveTime and KeepAliveInterval to 15 seconds.
		// Now we will use the default of KeepAliveInterval on Windows if user doesn't
		// provide one.
		interval = defaultKeepAliveInterval
	case idle < 0 && interval < 0:
		// Nothing to do, just bail out.
		return nil
	case idle >= 0 && interval >= 0:
		// Go ahead.
	}

	if idle == 0 {
		idle = defaultTCPKeepAliveIdle
	}
	if interval == 0 {
		interval = defaultTCPKeepAliveInterval
	}

	tcpKeepAliveIdle := uint32(roundDurationUp(idle, time.Millisecond))
	tcpKeepAliveInterval := uint32(roundDurationUp(interval, time.Millisecond))
	ka := windows.TCPKeepalive{
		OnOff:    1,
		Time:     tcpKeepAliveIdle,
		Interval: tcpKeepAliveInterval,
	}
	ret := uint32(0)
	size := uint32(unsafe.Sizeof(ka))
	handle := windows.Handle(fd.Fd())
	err := windows.WSAIoctl(handle, windows.SIO_KEEPALIVE_VALS, (*byte)(unsafe.Pointer(&ka)), size, nil, 0, &ret, nil, 0)
	runtime.KeepAlive(fd)
	return os.NewSyscallError("wsaioctl", err)
}

var (
	supportTCPKeepAliveIdle     bool
	supportTCPKeepAliveInterval bool
)

type _OSVERSIONINFOW struct {
	osVersionInfoSize uint32
	majorVersion      uint32
	minorVersion      uint32
	buildNumber       uint32
	platformId        uint32
	csdVersion        [128]uint16
}

var (
	modntdll          = syscall.NewLazyDLL("ntdll.dll")
	procRtlGetVersion = modntdll.NewProc("RtlGetVersion")
)

func rtlGetVersion(info *_OSVERSIONINFOW) {
	syscall.SyscallN(procRtlGetVersion.Addr(), 1, uintptr(unsafe.Pointer(info)), 0, 0)
	return
}
func version() (major, minor, build uint32) {
	info := _OSVERSIONINFOW{}
	info.osVersionInfoSize = uint32(unsafe.Sizeof(info))
	rtlGetVersion(&info)
	return info.majorVersion, info.minorVersion, info.buildNumber
}

var initTCPKeepAlive = sync.OnceFunc(func() {
	s, err := windows.WSASocket(windows.AF_INET, windows.SOCK_STREAM, windows.IPPROTO_TCP, nil, 0, windows.WSA_FLAG_NO_HANDLE_INHERIT)
	if err != nil {
		// Fallback to checking the Windows version.
		major, _, build := version()
		supportTCPKeepAliveIdle = major >= 10 && build >= 16299
		supportTCPKeepAliveInterval = major >= 10 && build >= 16299
		return
	}
	defer windows.Closesocket(s)
	var optSupported = func(opt int) bool {
		err := windows.SetsockoptInt(s, windows.IPPROTO_TCP, opt, 1)
		return !errors.Is(err, windows.WSAENOPROTOOPT)
	}
	supportTCPKeepAliveIdle = optSupported(windows.TCP_KEEPIDLE)
	supportTCPKeepAliveInterval = optSupported(windows.TCP_KEEPINTVL)
})

func SupportTCPKeepAliveIdle() bool {
	initTCPKeepAlive()
	return supportTCPKeepAliveIdle
}

func SupportTCPKeepAliveInterval() bool {
	initTCPKeepAlive()
	return supportTCPKeepAliveInterval
}
