//go:build windows

package aio

import (
	"errors"
	"golang.org/x/sys/windows"
	"net"
	"os"
	"syscall"
)

func connect(network string, family int, sotype int, proto int, raddr net.Addr, laddr net.Addr, cb OperationCallback) {
	// stream
	if sotype == syscall.SOCK_STREAM {
		connectEx(network, family, sotype, proto, raddr, cb)
		return
	}
	// packet

	// create sock
	sock, sockErr := newSocket(family, sotype, proto)
	if sockErr != nil {
		cb(0, Userdata{}, sockErr)
		return
	}
	handle := syscall.Handle(sock)

	// try set SO_BROADCAST
	if sotype == syscall.SOCK_DGRAM && (family == syscall.AF_INET || family == syscall.AF_INET6) {
		setBroadcastErr := syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
		if setBroadcastErr != nil {
			_ = syscall.Closesocket(handle)
			cb(0, Userdata{}, os.NewSyscallError("setsockopt", setBroadcastErr))
			return
		}
	}
	// local addr
	if laddr != nil {
		lsa := AddrToSockaddr(laddr)
		bindErr := syscall.Bind(handle, lsa)
		if bindErr != nil {
			_ = syscall.Closesocket(handle)
			cb(0, Userdata{}, os.NewSyscallError("bind", bindErr))
			return
		}
	}
	// connect
	rsa := AddrToSockaddr(raddr)
	connectErr := syscall.Connect(handle, rsa)
	if connectErr != nil {
		_ = syscall.Closesocket(handle)
		cb(0, Userdata{}, os.NewSyscallError("connect", connectErr))
		return
	}
	// get local addr
	if laddr == nil {
		lsa, lsaErr := syscall.Getsockname(handle)
		if lsaErr != nil {
			_ = syscall.Closesocket(handle)
			cb(0, Userdata{}, os.NewSyscallError("getsockname", lsaErr))
			return
		}
		laddr = SockaddrToAddr(network, lsa)
	}

	// create iocp
	createIOCPErr := createSubIoCompletionPort(windows.Handle(sock))
	if createIOCPErr != nil {
		_ = syscall.Closesocket(handle)
		cb(0, Userdata{}, createIOCPErr)
		return
	}

	// net fd
	nfd := &netFd{
		handle:     sock,
		network:    network,
		family:     family,
		socketType: sotype,
		protocol:   proto,
		localAddr:  laddr,
		remoteAddr: raddr,
		rop:        Operator{},
		wop:        Operator{},
	}
	nfd.rop.fd = nfd
	nfd.wop.fd = nfd

	userdata := Userdata{
		Fd: nfd,
	}
	cb(sock, userdata, nil)
	return
}

func connectEx(network string, family int, sotype int, proto int, addr net.Addr, cb OperationCallback) {
	sock, sockErr := newSocket(family, sotype, proto)
	if sockErr != nil {
		cb(0, Userdata{}, sockErr)
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
		cb(0, Userdata{}, os.NewSyscallError("bind", bindErr))
		return
	}
	// create iocp
	createIOCPErr := createSubIoCompletionPort(windows.Handle(sock))
	if createIOCPErr != nil {
		_ = syscall.Closesocket(handle)
		cb(0, Userdata{}, createIOCPErr)
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
		localAddr:  nil,
		remoteAddr: addr,
		rop:        Operator{},
		wop:        Operator{},
	}
	nfd.rop.fd = nfd
	nfd.wop.fd = nfd

	// op
	op := nfd.ReadOperator()
	op.userdata.Fd = nfd
	// callback
	op.callback = cb
	// completion
	op.completion = completeConnectEx

	// overlapped
	overlapped := &op.overlapped

	// timeout
	if op.timeout > 0 {
		timer := getOperatorTimer()
		op.timer = timer
		timer.Start(op.timeout, &operatorCanceler{
			handle:     syscall.Handle(sock),
			overlapped: overlapped,
		})
	}

	// connect
	connectErr := syscall.ConnectEx(handle, sa, nil, 0, nil, overlapped)
	if connectErr != nil && !errors.Is(connectErr, syscall.ERROR_IO_PENDING) {
		_ = syscall.Closesocket(handle)
		cb(0, op.userdata, os.NewSyscallError("connectex", connectErr))
		op.callback = nil
		op.completion = nil
		if op.timer != nil {
			timer := op.timer
			timer.Done()
			putOperatorTimer(timer)
			op.timer = nil
		}
		return
	}
	return
}

func completeConnectEx(result int, op *Operator, err error) {
	nfd := op.fd.(*netFd)
	handle := syscall.Handle(nfd.Fd())
	if err != nil {
		_ = syscall.Closesocket(handle)
		op.callback(result, op.userdata, os.NewSyscallError("connectex", err))
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
		op.callback(result, op.userdata, os.NewSyscallError("setsockopt", setSocketOptErr))
		return
	}
	// get addr
	lsa, lsaErr := syscall.Getsockname(handle)
	if lsaErr != nil {
		_ = syscall.Closesocket(handle)
		op.callback(result, op.userdata, os.NewSyscallError("getsockname", lsaErr))
		return
	}
	la := SockaddrToAddr(nfd.Network(), lsa)
	nfd.localAddr = la
	rsa, rsaErr := syscall.Getpeername(handle)
	if rsaErr != nil {
		_ = syscall.Closesocket(handle)
		op.callback(result, op.userdata, os.NewSyscallError("getsockname", rsaErr))
		return
	}
	ra := SockaddrToAddr(nfd.Network(), rsa)
	nfd.remoteAddr = ra

	// callback
	op.callback(nfd.Fd(), op.userdata, nil)
}
