//go:build windows

package aio

import (
	"golang.org/x/sys/windows"
	"net"
	"os"
	"syscall"
)

func newListenerFd(network string, family int, sotype int, proto int, ipv6only bool, addr net.Addr, multicastInterface *net.Interface) (v *netFd, err error) {
	// create sock
	sock, sockErr := newSocket(family, sotype, proto, ipv6only)
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
		ipv6only:   ipv6only,
		localAddr:  addr,
		remoteAddr: nil,
		rop:        nil,
		wop:        nil,
	}
	nfd.rop = newOperator(nfd)
	nfd.wop = newOperator(nfd)

	v = nfd
	return
}
