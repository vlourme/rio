//go:build windows

package aio

import (
	"errors"
	"golang.org/x/sys/windows"
	"net"
	"os"
	"syscall"
	"unsafe"
)

func newListenerFd(network string, family int, sotype int, proto int, addr net.Addr, multicastInterface *net.Interface) (v *netFd, err error) {
	// create sock
	sock, sockErr := newSocket(family, sotype, proto)
	if sockErr != nil {
		err = sockErr
		return
	}
	handle := syscall.Handle(sock)
	switch sotype {
	case syscall.SOCK_STREAM, syscall.SOCK_SEQPACKET:
		setOptErr := setDefaultListenerSocketOpts(sock)
		if setOptErr != nil {
			_ = syscall.Closesocket(handle)
			err = setOptErr
			return
		}
		// bind
		sa := AddrToSockaddr(addr)
		bindErr := syscall.Bind(handle, sa)
		if bindErr != nil {
			err = os.NewSyscallError("bind", bindErr)
			_ = syscall.Closesocket(handle)
			return
		}
		// listen
		listenErr := syscall.Listen(handle, syscall.SOMAXCONN)
		if listenErr != nil {
			err = os.NewSyscallError("listen", listenErr)
			_ = syscall.Closesocket(handle)
			return
		}
		// lsa
		lsa, getLsaErr := syscall.Getsockname(handle)
		if getLsaErr != nil {
			err = os.NewSyscallError("getsockname", getLsaErr)
			_ = syscall.Closesocket(handle)
			return
		}
		addr = SockaddrToAddr(network, lsa)
		break
	case syscall.SOCK_DGRAM:
		isListenMulticastUDP := false
		var gaddr *net.UDPAddr
		udpAddr, isUdpAddr := addr.(*net.UDPAddr)
		if isUdpAddr {
			if udpAddr.IP != nil && udpAddr.IP.IsMulticast() {
				if setOptErr := setDefaultMulticastSockopts(sock); setOptErr != nil {
					_ = syscall.Close(handle)
					err = setOptErr
					return
				}
				isListenMulticastUDP = true
				gaddr = udpAddr
				localUdpAddr := *udpAddr
				switch family {
				case syscall.AF_INET:
					localUdpAddr.IP = net.IPv4zero.To4()
				case syscall.AF_INET6:
					localUdpAddr.IP = net.IPv6zero
				}
				addr = &localUdpAddr
			}
		}
		// listen multicast udp
		if isListenMulticastUDP {
			if ip4 := gaddr.IP.To4(); ip4 != nil {
				if multicastInterface != nil {
					if err = setIPv4MulticastInterface(handle, multicastInterface); err != nil {
						_ = syscall.Closesocket(handle)
						return
					}
				}
				if err = setIPv4MulticastLoopback(handle, false); err != nil {
					_ = syscall.Closesocket(handle)
					return
				}
				if err = joinIPv4Group(handle, multicastInterface, ip4); err != nil {
					_ = syscall.Closesocket(handle)
					return
				}
			} else {
				if multicastInterface != nil {
					if err = setIPv6MulticastInterface(handle, multicastInterface); err != nil {
						_ = syscall.Closesocket(handle)
						return
					}
				}
				if err = setIPv6MulticastLoopback(handle, false); err != nil {
					_ = syscall.Closesocket(handle)
					return
				}
				if err = joinIPv6Group(handle, multicastInterface, gaddr.IP); err != nil {
					_ = syscall.Closesocket(handle)
					return
				}
			}
		}
		// bind
		sa := AddrToSockaddr(addr)
		bindErr := syscall.Bind(handle, sa)
		if bindErr != nil {
			err = os.NewSyscallError("bind", bindErr)
			_ = syscall.Closesocket(handle)
			return
		}
		break
	default:
		break
	}

	// create iocp
	createListenIOCPErr := createSubIoCompletionPort(windows.Handle(sock))
	if createListenIOCPErr != nil {
		err = createListenIOCPErr
		_ = syscall.Closesocket(handle)
		return
	}

	// fd
	nfd := &netFd{
		handle:     sock,
		network:    network,
		family:     family,
		socketType: sotype,
		protocol:   proto,
		localAddr:  addr,
		remoteAddr: nil,
		rop:        Operator{},
		wop:        Operator{},
	}
	nfd.rop.fd = nfd
	nfd.wop.fd = nfd

	v = nfd
	return
}

func Accept(fd NetFd, cb OperationCallback) {
	// conn
	sock, sockErr := newSocket(fd.Family(), fd.SocketType(), fd.Protocol())
	if sockErr != nil {
		cb(0, Userdata{}, errors.Join(errors.New("aio: accept failed"), sockErr))
		return
	}
	// op
	op := fd.ReadOperator()
	op.userdata.Fd = &netFd{
		handle:     sock,
		network:    fd.Network(),
		family:     fd.Family(),
		socketType: fd.SocketType(),
		protocol:   fd.Protocol(),
		localAddr:  nil,
		remoteAddr: nil,
	}
	// callback
	op.callback = cb
	// completion
	op.completion = completeAccept

	// overlapped
	overlapped := &op.overlapped

	// sa
	var rawsa [2]syscall.RawSockaddrAny
	lsan := uint32(unsafe.Sizeof(rawsa[1]))
	rsa := &rawsa[0]
	rsan := uint32(unsafe.Sizeof(rawsa[0]))

	// timeout
	if op.timeout > 0 {
		timer := getOperatorTimer()
		op.timer = timer
		timer.Start(op.timeout, &operatorCanceler{
			handle:     syscall.Handle(sock),
			overlapped: overlapped,
		})
	}

	// accept
	acceptErr := syscall.AcceptEx(
		syscall.Handle(fd.Fd()), syscall.Handle(sock),
		(*byte)(unsafe.Pointer(rsa)), 0,
		lsan+16, rsan+16,
		&op.userdata.QTY, overlapped,
	)
	if acceptErr != nil && !errors.Is(syscall.ERROR_IO_PENDING, acceptErr) {
		_ = syscall.Closesocket(syscall.Handle(sock))
		cb(0, op.userdata, os.NewSyscallError("acceptex", acceptErr))

		op.callback = nil
		op.completion = nil
		if op.timer != nil {
			timer := op.timer
			timer.Done()
			putOperatorTimer(timer)
			op.timer = nil
		}
	}

	return
}

func completeAccept(result int, op *Operator, err error) {
	userdata := op.userdata
	// conn
	conn, _ := userdata.Fd.(*netFd)
	connFd := syscall.Handle(conn.handle)
	if err != nil {
		_ = syscall.Closesocket(connFd)
		op.callback(result, userdata, os.NewSyscallError("acceptex", err))
		return
	}
	// ln
	ln, _ := op.fd.(NetFd)
	lnFd := syscall.Handle(ln.Fd())

	// set SO_UPDATE_ACCEPT_CONTEXT
	setAcceptSocketOptErr := syscall.Setsockopt(
		connFd,
		windows.SOL_SOCKET, windows.SO_UPDATE_ACCEPT_CONTEXT,
		(*byte)(unsafe.Pointer(&lnFd)),
		int32(unsafe.Sizeof(lnFd)),
	)
	if setAcceptSocketOptErr != nil {
		_ = syscall.Closesocket(connFd)
		op.callback(result, userdata, os.NewSyscallError("setsockopt", setAcceptSocketOptErr))
		return
	}

	// get local addr
	lsa, lsaErr := syscall.Getsockname(connFd)
	if lsaErr != nil {
		_ = syscall.Closesocket(connFd)
		op.callback(result, userdata, os.NewSyscallError("getsockname", lsaErr))
		return
	}
	la := SockaddrToAddr(ln.Network(), lsa)
	conn.localAddr = la

	// get remote addr
	rsa, rsaErr := syscall.Getpeername(connFd)
	if rsaErr != nil {
		_ = syscall.Closesocket(connFd)
		op.callback(result, userdata, os.NewSyscallError("getsockname", rsaErr))
		return
	}
	ra := SockaddrToAddr(ln.Network(), rsa)
	conn.remoteAddr = ra

	// create iocp
	iocpErr := createSubIoCompletionPort(windows.Handle(connFd))
	if iocpErr != nil {
		_ = syscall.Closesocket(connFd)
		op.callback(result, userdata, iocpErr)
		return
	}

	// callback
	op.callback(conn.handle, userdata, err)
	return
}
