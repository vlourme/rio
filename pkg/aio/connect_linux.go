//go:build linux

package aio

import (
	"net"
	"os"
	"syscall"
	"unsafe"
)

func connect(network string, family int, sotype int, proto int, ipv6only bool, raddr net.Addr, laddr net.Addr, cb OperationCallback) {
	// create sock
	sock, sockErr := newSocket(family, sotype, proto, ipv6only)
	if sockErr != nil {
		cb(Userdata{}, sockErr)
		return
	}

	// try set SO_BROADCAST
	if sotype == syscall.SOCK_DGRAM && (family == syscall.AF_INET || family == syscall.AF_INET6) {
		setBroadcastErr := syscall.SetsockoptInt(sock, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
		if setBroadcastErr != nil {
			_ = syscall.Close(sock)
			cb(Userdata{}, os.NewSyscallError("setsockopt", setBroadcastErr))
			return
		}
	}

	// conn
	conn := newNetFd(sock, network, family, sotype, proto, ipv6only, nil, nil)

	// local addr
	if laddr != nil {
		lsa := AddrToSockaddr(laddr)
		bindErr := syscall.Bind(sock, lsa)
		if bindErr != nil {
			_ = syscall.Close(sock)
			cb(Userdata{}, os.NewSyscallError("bind", bindErr))
			return
		}
		conn.localAddr = laddr
		if raddr == nil {
			cb(Userdata{Fd: conn}, nil)
			return
		}
	}
	// remote addr
	if raddr == nil {
		_ = syscall.Close(sock)
		cb(Userdata{}, syscall.Errno(22))
		return
	}
	sa := AddrToSockaddr(raddr)
	rsa, rsaLen, rsaErr := SockaddrToRaw(sa)
	if rsaErr != nil {
		cb(Userdata{}, rsaErr)
		return
	}
	conn.remoteAddr = raddr
	// op
	op := acquireOperator(conn)

	// cb
	op.callback = cb
	// completion
	op.completion = completeConnect
	// cylinder
	cylinder := nextIOURingCylinder()
	op.setCylinder(cylinder)

	// prepare
	err := cylinder.prepareRW(opConnect, sock, uintptr(unsafe.Pointer(rsa)), 0, uint64(rsaLen), 0, op.ptr())
	if err != nil {
		_ = syscall.Close(sock)
		cb(Userdata{}, os.NewSyscallError("io_uring_prep_connect", err))
		// release
		releaseOperator(op)
	}
	return
}

func completeConnect(_ int, op *Operator, err error) {
	cb := op.callback
	conn := op.fd.(*netFd)

	releaseOperator(op)

	sock := conn.Fd()
	// check error
	if err != nil {
		_ = syscall.Close(sock)
		cb(Userdata{}, os.NewSyscallError("io_uring_prep_connect", err))
		return
	}
	// get local addr
	if conn.localAddr == nil {
		lsa, lsaErr := syscall.Getsockname(sock)
		if lsaErr != nil {
			_ = syscall.Close(sock)
			cb(Userdata{}, os.NewSyscallError("getsockname", lsaErr))
			return
		}
		la := SockaddrToAddr(conn.Network(), lsa)
		conn.localAddr = la
	}
	// callback
	cb(Userdata{Fd: conn}, nil)
	return
}
