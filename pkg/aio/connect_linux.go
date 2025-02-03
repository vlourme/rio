//go:build linux

package aio

import (
	"net"
	"os"
	"runtime"
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

	// net fd
	nfd := &netFd{
		handle:     sock,
		network:    network,
		family:     family,
		socketType: sotype,
		protocol:   proto,
		ipv6only:   ipv6only,
		localAddr:  nil,
		remoteAddr: nil,
		rop:        nil,
		wop:        nil,
	}
	nfd.rop = newOperator(nfd)
	nfd.wop = newOperator(nfd)

	// local addr
	if laddr != nil {
		lsa := AddrToSockaddr(laddr)
		bindErr := syscall.Bind(sock, lsa)
		if bindErr != nil {
			_ = syscall.Close(sock)
			cb(Userdata{}, os.NewSyscallError("bind", bindErr))
			return
		}
		nfd.localAddr = laddr
		if raddr == nil {
			cb(Userdata{Fd: nfd}, nil)
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
	nfd.remoteAddr = raddr
	// op
	op := nfd.ReadOperator()

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
		// reset
		op.reset()
	}
	return
}

func completeConnect(_ int, op *Operator, err error) {
	cb := op.callback
	conn := op.fd.(*netFd)
	connFd := conn.Fd()
	// check error
	if err != nil {
		_ = syscall.Close(connFd)
		cb(Userdata{}, os.NewSyscallError("io_uring_prep_connect", err))
		return
	}
	// get local addr
	if conn.localAddr == nil {
		lsa, lsaErr := syscall.Getsockname(connFd)
		if lsaErr != nil {
			_ = syscall.Close(connFd)
			cb(Userdata{}, os.NewSyscallError("getsockname", lsaErr))
			return
		}
		la := SockaddrToAddr(conn.Network(), lsa)
		conn.localAddr = la
	}
	// callback
	cb(Userdata{Fd: conn}, nil)
	runtime.KeepAlive(cb)
	return
}
