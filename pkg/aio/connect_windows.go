//go:build windows

package aio

import (
	"github.com/brickingsoft/errors"
	"golang.org/x/sys/windows"
	"net"
	"os"
	"syscall"
)

func connect(network string, family int, sotype int, proto int, ipv6only bool, raddr net.Addr, laddr net.Addr, fastOpen int, cb OperationCallback) {
	// stream
	if sotype == syscall.SOCK_STREAM {
		connectEx(network, family, sotype, proto, ipv6only, raddr, fastOpen, cb)
		return
	}
	// packet

	// create sock
	sock, sockErr := newSocket(family, sotype, proto, ipv6only)
	if sockErr != nil {
		err := errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(sockErr),
		)
		cb(Userdata{}, err)
		return
	}
	handle := syscall.Handle(sock)

	// try set SO_BROADCAST
	if sotype == syscall.SOCK_DGRAM && (family == syscall.AF_INET || family == syscall.AF_INET6) {
		setBroadcastErr := syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
		if setBroadcastErr != nil {
			_ = syscall.Closesocket(handle)
			err := errors.New(
				"connect failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpConnect),
				errors.WithWrap(os.NewSyscallError("setsockopt", setBroadcastErr)),
			)
			cb(Userdata{}, err)
			return
		}
	}
	// local addr
	if laddr != nil {
		lsa := AddrToSockaddr(laddr)
		bindErr := syscall.Bind(handle, lsa)
		if bindErr != nil {
			_ = syscall.Closesocket(handle)
			err := errors.New(
				"connect failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpConnect),
				errors.WithWrap(os.NewSyscallError("bind", bindErr)),
			)
			cb(Userdata{}, err)
			return
		}
	}
	// connect
	rsa := AddrToSockaddr(raddr)
	connectErr := syscall.Connect(handle, rsa)
	if connectErr != nil {
		_ = syscall.Closesocket(handle)
		err := errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(os.NewSyscallError("connect", connectErr)),
		)
		cb(Userdata{}, err)
		return
	}
	// get local addr
	if laddr == nil {
		lsa, lsaErr := syscall.Getsockname(handle)
		if lsaErr != nil {
			_ = syscall.Closesocket(handle)
			err := errors.New(
				"connect failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpConnect),
				errors.WithWrap(os.NewSyscallError("getsockname", lsaErr)),
			)
			cb(Userdata{}, err)
			return
		}
		laddr = SockaddrToAddr(network, lsa)
	}

	// create iocp
	createIOCPErr := createSubIoCompletionPort(windows.Handle(sock))
	if createIOCPErr != nil {
		_ = syscall.Closesocket(handle)
		err := errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(createIOCPErr),
		)
		cb(Userdata{}, err)
		return
	}

	// net fd
	conn := newNetFd(sock, network, family, sotype, proto, ipv6only, laddr, raddr)
	cb(Userdata{Fd: conn}, nil)
	return
}

func connectEx(network string, family int, sotype int, proto int, ipv6only bool, addr net.Addr, fastOpen int, cb OperationCallback) {
	sock, sockErr := newSocket(family, sotype, proto, ipv6only)
	if sockErr != nil {
		err := errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(sockErr),
		)
		cb(Userdata{}, err)
		return
	}
	// conn
	conn := newNetFd(sock, network, family, sotype, proto, ipv6only, nil, addr)
	// fast open
	if fastOpen > 0 {
		_ = SetFastOpen(conn, fastOpen)
	}
	// sock
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
		err := errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(os.NewSyscallError("bind", bindErr)),
		)
		cb(Userdata{}, err)
		return
	}
	// create iocp
	createIOCPErr := createSubIoCompletionPort(windows.Handle(sock))
	if createIOCPErr != nil {
		_ = syscall.Closesocket(handle)
		err := errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(createIOCPErr),
		)
		cb(Userdata{}, err)
		return
	}
	// remote addr
	sa := AddrToSockaddr(addr)

	// op
	op := acquireOperator(conn)
	// callback
	op.callback = cb
	// completion
	op.completion = completeConnectEx

	// overlapped
	overlapped := &op.overlapped

	// connect
	connectErr := syscall.ConnectEx(handle, sa, nil, 0, nil, overlapped)
	if connectErr != nil && !errors.Is(connectErr, syscall.ERROR_IO_PENDING) {
		_ = syscall.Closesocket(handle)
		err := errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(os.NewSyscallError("connectex", connectErr)),
		)
		cb(Userdata{}, err)
		releaseOperator(op)
		return
	}
	return
}

func completeConnectEx(_ int, op *Operator, err error) {
	cb := op.callback

	fd := op.fd.(*netFd)
	handle := syscall.Handle(fd.Fd())

	releaseOperator(op)

	if err != nil {
		_ = syscall.Closesocket(handle)
		err = errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(os.NewSyscallError("connectex", err)),
		)
		cb(Userdata{}, err)
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
		err = errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(os.NewSyscallError("setsockopt", setSocketOptErr)),
		)
		cb(Userdata{}, err)
		return
	}
	// get addr
	lsa, lsaErr := syscall.Getsockname(handle)
	if lsaErr != nil {
		_ = syscall.Closesocket(handle)
		err = errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(os.NewSyscallError("getsockname", lsaErr)),
		)
		cb(Userdata{}, err)
		return
	}
	la := SockaddrToAddr(fd.Network(), lsa)
	fd.localAddr = la
	rsa, rsaErr := syscall.Getpeername(handle)
	if rsaErr != nil {
		_ = syscall.Closesocket(handle)
		err = errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(os.NewSyscallError("getsockname", rsaErr)),
		)
		cb(Userdata{}, err)
		return
	}
	ra := SockaddrToAddr(fd.Network(), rsa)
	fd.remoteAddr = ra

	// callback
	cb(Userdata{Fd: fd}, nil)
}
