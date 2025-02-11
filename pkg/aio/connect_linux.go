//go:build linux

package aio

import (
	"github.com/brickingsoft/errors"
	"net"
	"os"
	"syscall"
	"unsafe"
)

func connect(network string, family int, sotype int, proto int, ipv6only bool, raddr net.Addr, laddr net.Addr, fastOpen int, cb OperationCallback) {
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

	// try set SO_BROADCAST
	if sotype == syscall.SOCK_DGRAM && (family == syscall.AF_INET || family == syscall.AF_INET6) {
		setBroadcastErr := syscall.SetsockoptInt(sock, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
		if setBroadcastErr != nil {
			_ = syscall.Close(sock)
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
	// cylinder
	cylinder := nextIOURingCylinder()
	// conn
	conn := newNetFd(cylinder, sock, network, family, sotype, proto, ipv6only, nil, nil)
	// fast open
	if fastOpen > 0 {
		_ = SetFastOpen(conn, fastOpen)
	}

	// local addr
	if laddr != nil {
		lsa := AddrToSockaddr(laddr)
		bindErr := syscall.Bind(sock, lsa)
		if bindErr != nil {
			_ = syscall.Close(sock)
			err := errors.New(
				"connect failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpConnect),
				errors.WithWrap(os.NewSyscallError("bind", bindErr)),
			)
			cb(Userdata{}, err)
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
		err := errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(syscall.Errno(22)),
		)
		cb(Userdata{}, err)
		return
	}
	sa := AddrToSockaddr(raddr)
	rsa, rsaLen, rsaErr := SockaddrToRaw(sa)
	if rsaErr != nil {
		err := errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(rsaErr),
		)
		cb(Userdata{}, err)
		return
	}
	conn.remoteAddr = raddr
	// op
	op := acquireOperator(conn)
	// cb
	op.callback = cb
	// completion
	op.completion = completeConnect

	// prepare
	err := cylinder.prepareRW(opConnect, sock, uintptr(unsafe.Pointer(rsa)), 0, uint64(rsaLen), 0, op.ptr())
	if err != nil {
		releaseOperator(op)
		_ = syscall.Close(sock)
		err = errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(os.NewSyscallError("io_uring_prep_connect", err)),
		)
		cb(Userdata{}, err)
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
		err = errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(err),
		)
		cb(Userdata{}, err)
		return
	}
	// get local addr
	if conn.localAddr == nil {
		lsa, lsaErr := syscall.Getsockname(sock)
		if lsaErr != nil {
			_ = syscall.Close(sock)
			err = errors.New(
				"connect failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpConnect),
				errors.WithWrap(os.NewSyscallError("getsockname", lsaErr)),
			)
			cb(Userdata{}, err)
			return
		}
		la := SockaddrToAddr(conn.Network(), lsa)
		conn.localAddr = la
	}
	// callback
	cb(Userdata{Fd: conn}, nil)
	return
}
