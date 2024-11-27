//go:build windows

package aio

import (
	"golang.org/x/sys/windows"
	"net"
	"os"
	"syscall"
)

func newNetFd(network string, family int, sotype int, proto int, laddr net.Addr, raddr net.Addr, multicastInterface *net.Interface) (v *netFd, err error) {
	// create sock
	sock, sockErr := newSocket(family, sotype, proto)
	if sockErr != nil {
		err = sockErr
		return
	}
	handle := syscall.Handle(sock)

	// try connect
	if raddr != nil { // new packet conn
		// try set SO_BROADCAST
		if sotype == syscall.SOCK_DGRAM && (family == syscall.AF_INET || family == syscall.AF_INET6) {
			setBroadcaseErr := syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
			if setBroadcaseErr != nil {
				err = os.NewSyscallError("setsockopt", setBroadcaseErr)
				_ = syscall.Closesocket(handle)
				return
			}
		}

		// connect
		sa := AddrToSockaddr(raddr)
		connectErr := syscall.Connect(handle, sa)
		if connectErr != nil {
			err = os.NewSyscallError("connect", connectErr)
			_ = syscall.Closesocket(handle)
			return
		}
		lsa, _ := syscall.Getsockname(handle)
		laddr = SockaddrToAddr(network, lsa)
	}

	// listen
	if laddr != nil {
		// set default ln opts
		setOptErr := setDefaultListenerSocketOpts(sock)
		if setOptErr != nil {
			_ = syscall.Closesocket(handle)
			err = setOptErr
			return
		}
		// check multicast
		isListenMulticastUDP := false
		var gaddr *net.UDPAddr
		switch addr := laddr.(type) {
		case *net.UDPAddr:
			if addr.IP != nil && addr.IP.IsMulticast() {
				isListenMulticastUDP = true
				gaddr = addr
				localUdpAddr := *addr
				switch family {
				case syscall.AF_INET:
					localUdpAddr.IP = net.IPv4zero.To4()
				case syscall.AF_INET6:
					localUdpAddr.IP = net.IPv6zero
				}
				laddr = &localUdpAddr
			}
			break
		default:
			break
		}

		// bind
		sa := AddrToSockaddr(laddr)
		bindErr := syscall.Bind(handle, sa)
		if bindErr != nil {
			err = os.NewSyscallError("bind", bindErr)
			_ = syscall.Closesocket(handle)
			return
		}

		// listen multicast udp
		if isListenMulticastUDP && gaddr != nil {
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
		} else {
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
			laddr = SockaddrToAddr(network, lsa)
		}
	}

	// create iocp
	_, createListenIOCPErr := createSubIoCompletionPort(windows.Handle(sock))
	if createListenIOCPErr != nil {
		err = os.NewSyscallError("CreateIoCompletionPort", createListenIOCPErr)
		_ = syscall.Closesocket(handle)
		return
	}

	// fd
	sfd := &netFd{
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
	sfd.rop.fd = sfd
	sfd.wop.fd = sfd

	// return
	v = sfd
	return
}
