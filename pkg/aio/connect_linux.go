//go:build linux

package aio

import (
	"net"
	"os"
	"syscall"
	"unsafe"
)

func connect(network string, family int, sotype int, proto int, raddr net.Addr, laddr net.Addr, cb OperationCallback) {
	// stream
	if sotype == syscall.SOCK_STREAM {
		connectAsync(network, family, sotype, proto, raddr, cb)
		return
	}
	// packet

	// create sock
	sock, sockErr := newSocket(family, sotype, proto)
	if sockErr != nil {
		cb(0, Userdata{}, sockErr)
		return
	}
	// try set SO_BROADCAST
	if sotype == syscall.SOCK_DGRAM && (family == syscall.AF_INET || family == syscall.AF_INET6) {
		setBroadcastErr := syscall.SetsockoptInt(sock, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
		if setBroadcastErr != nil {
			_ = syscall.Close(sock)
			cb(0, Userdata{}, os.NewSyscallError("setsockopt", setBroadcastErr))
			return
		}
	}
	// local addr
	if laddr != nil {
		lsa := AddrToSockaddr(laddr)
		bindErr := syscall.Bind(sock, lsa)
		if bindErr != nil {
			_ = syscall.Close(sock)
			cb(0, Userdata{}, os.NewSyscallError("bind", bindErr))
			return
		}
	}
	// connect
	rsa := AddrToSockaddr(raddr)
	connectErr := syscall.Connect(sock, rsa)
	if connectErr != nil {
		_ = syscall.Close(sock)
		cb(0, Userdata{}, os.NewSyscallError("connect", connectErr))
		return
	}
	// get local addr
	if laddr == nil {
		lsa, lsaErr := syscall.Getsockname(sock)
		if lsaErr != nil {
			_ = syscall.Close(sock)
			cb(0, Userdata{}, os.NewSyscallError("getsockname", lsaErr))
			return
		}
		laddr = SockaddrToAddr(network, lsa)
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

func connectAsync(network string, family int, sotype int, proto int, addr net.Addr, cb OperationCallback) {
	sock, sockErr := newSocket(family, sotype, proto)
	if sockErr != nil {
		cb(0, Userdata{}, sockErr)
		return
	}

	sa := AddrToSockaddr(addr)
	rsa, rsaLen, rsaErr := SockaddrToRaw(sa)
	if rsaErr != nil {
		cb(0, Userdata{}, rsaErr)
		return
	}

	// net fd
	nfd := &netFd{
		handle:     sock,
		network:    network,
		family:     family,
		socketType: sock,
		protocol:   proto,
		localAddr:  nil,
		remoteAddr: addr,
		rop:        Operator{},
		wop:        Operator{},
	}
	nfd.rop.fd = nfd
	nfd.wop.fd = nfd

	// op
	op := nfd.rop
	op.userdata.Fd = nfd
	// cb
	op.callback = cb
	// completion
	op.completion = completeConnectAsync
	// userdata
	userdata := uint64(uintptr(unsafe.Pointer(&op)))
	// prepare
	err := prepare(opConnect, sock, uintptr(unsafe.Pointer(rsa)), 0, uint64(rsaLen), 0, userdata)
	if err != nil {
		_ = syscall.Close(sock)
		cb(0, Userdata{}, os.NewSyscallError("io_uring_prep_connect", err))
		// reset
		op.callback = nil
		op.completion = nil
		return
	}
	return
}

func completeConnectAsync(result int, op *Operator, err error) {
	cb := op.callback
	conn := op.fd.(*netFd)
	connFd := conn.Fd()
	// check error
	if err != nil {
		_ = syscall.Close(connFd)
		cb(result, Userdata{}, os.NewSyscallError("io_uring_prep_connect", err))
		return
	}
	// get local addr
	lsa, lsaErr := syscall.Getsockname(connFd)
	if lsaErr != nil {
		_ = syscall.Close(connFd)
		cb(result, Userdata{}, os.NewSyscallError("getsockname", lsaErr))
		return
	}
	la := SockaddrToAddr(conn.Network(), lsa)
	conn.localAddr = la
	// callback
	cb(connFd, op.userdata, nil)
	return
}
