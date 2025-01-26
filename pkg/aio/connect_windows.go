//go:build windows

package aio

import (
	"errors"
	"golang.org/x/sys/windows"
	"net"
	"os"
	"syscall"
	"time"
)

func connect(network string, family int, sotype int, proto int, ipv6only bool, raddr net.Addr, laddr net.Addr, timeout time.Duration, cb OperationCallback) {
	// stream
	if sotype == syscall.SOCK_STREAM {
		connectEx(network, family, sotype, proto, ipv6only, raddr, timeout, cb)
		return
	}
	// packet

	// create sock
	sock, sockErr := newSocket(family, sotype, proto, ipv6only)
	if sockErr != nil {
		cb(Userdata{}, sockErr)
		return
	}
	handle := syscall.Handle(sock)

	// try set SO_BROADCAST
	if sotype == syscall.SOCK_DGRAM && (family == syscall.AF_INET || family == syscall.AF_INET6) {
		setBroadcastErr := syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
		if setBroadcastErr != nil {
			_ = syscall.Closesocket(handle)
			cb(Userdata{}, os.NewSyscallError("setsockopt", setBroadcastErr))
			return
		}
	}
	// local addr
	if laddr != nil {
		lsa := AddrToSockaddr(laddr)
		bindErr := syscall.Bind(handle, lsa)
		if bindErr != nil {
			_ = syscall.Closesocket(handle)
			cb(Userdata{}, os.NewSyscallError("bind", bindErr))
			return
		}
	}
	// connect
	rsa := AddrToSockaddr(raddr)
	connectErr := syscall.Connect(handle, rsa)
	if connectErr != nil {
		_ = syscall.Closesocket(handle)
		cb(Userdata{}, os.NewSyscallError("connect", connectErr))
		return
	}
	// get local addr
	if laddr == nil {
		lsa, lsaErr := syscall.Getsockname(handle)
		if lsaErr != nil {
			_ = syscall.Closesocket(handle)
			cb(Userdata{}, os.NewSyscallError("getsockname", lsaErr))
			return
		}
		laddr = SockaddrToAddr(network, lsa)
	}

	// create iocp
	createIOCPErr := createSubIoCompletionPort(windows.Handle(sock))
	if createIOCPErr != nil {
		_ = syscall.Closesocket(handle)
		cb(Userdata{}, createIOCPErr)
		return
	}

	// net fd
	nfd := &netFd{
		handle:     sock,
		network:    network,
		family:     family,
		socketType: sotype,
		protocol:   proto,
		ipv6only:   ipv6only,
		localAddr:  laddr,
		remoteAddr: raddr,
		rop:        nil,
		wop:        nil,
	}
	nfd.rop = newOperator(nfd)
	nfd.wop = newOperator(nfd)

	cb(Userdata{Fd: nfd}, nil)
	return
}

func connectEx(network string, family int, sotype int, proto int, ipv6only bool, addr net.Addr, timeout time.Duration, cb OperationCallback) {
	sock, sockErr := newSocket(family, sotype, proto, ipv6only)
	if sockErr != nil {
		cb(Userdata{}, sockErr)
		return
	}
	handle := syscall.Handle(sock)

	// lsa
	var lsa syscall.Sockaddr
	if family == syscall.AF_INET6 {
		lsa = &syscall.SockaddrInet6{}
	} else {
		lsa = &syscall.SockaddrInet4{}
	}
	// bind
	bindErr := syscall.Bind(handle, lsa)
	if bindErr != nil {
		_ = syscall.Closesocket(handle)
		cb(Userdata{}, os.NewSyscallError("bind", bindErr))
		return
	}
	// create iocp
	createIOCPErr := createSubIoCompletionPort(windows.Handle(sock))
	if createIOCPErr != nil {
		_ = syscall.Closesocket(handle)
		cb(Userdata{}, createIOCPErr)
		return
	}
	// remote addr
	sa := AddrToSockaddr(addr)
	// net fd
	nfd := &netFd{
		handle:     sock,
		network:    network,
		family:     family,
		socketType: sotype,
		protocol:   proto,
		ipv6only:   ipv6only,
		localAddr:  nil,
		remoteAddr: addr,
		rop:        nil,
		wop:        nil,
	}
	nfd.rop = newOperator(nfd)
	nfd.wop = newOperator(nfd)

	// op
	op := nfd.ReadOperator()
	op.fd = nfd
	// callback
	op.callback = cb
	// completion
	op.completion = completeConnectEx
	// timeout
	if timeout > 0 {
		op.timeout = timeout
		op.tryPrepareTimeout()
	}
	// overlapped
	overlapped := &op.overlapped

	// connect
	connectErr := syscall.ConnectEx(handle, sa, nil, 0, nil, overlapped)
	if connectErr != nil && !errors.Is(connectErr, syscall.ERROR_IO_PENDING) {
		_ = syscall.Closesocket(handle)
		cb(Userdata{}, os.NewSyscallError("connectex", connectErr))
		op.reset()
		return
	}
	return
}

func completeConnectEx(_ int, op *Operator, err error) {
	if op.timeout > 0 {
		op.timeout = 0
	}
	nfd := op.fd.(*netFd)
	handle := syscall.Handle(nfd.Fd())
	if err != nil {
		_ = syscall.Closesocket(handle)
		op.callback(Userdata{}, os.NewSyscallError("connectex", err))
		return
	}
	// set SO_UPDATE_CONNECT_CONTEXT
	setSocketOptErr := syscall.Setsockopt(
		handle,
		syscall.SOL_SOCKET, syscall.SO_UPDATE_CONNECT_CONTEXT,
		nil,
		0,
	)
	if setSocketOptErr != nil {
		_ = syscall.Closesocket(handle)
		op.callback(Userdata{}, os.NewSyscallError("setsockopt", setSocketOptErr))
		return
	}
	// get addr
	lsa, lsaErr := syscall.Getsockname(handle)
	if lsaErr != nil {
		_ = syscall.Closesocket(handle)
		op.callback(Userdata{}, os.NewSyscallError("getsockname", lsaErr))
		return
	}
	la := SockaddrToAddr(nfd.Network(), lsa)
	nfd.localAddr = la
	rsa, rsaErr := syscall.Getpeername(handle)
	if rsaErr != nil {
		_ = syscall.Closesocket(handle)
		op.callback(Userdata{}, os.NewSyscallError("getsockname", rsaErr))
		return
	}
	ra := SockaddrToAddr(nfd.Network(), rsa)
	nfd.remoteAddr = ra

	// callback
	op.callback(Userdata{Fd: nfd}, nil)
}
