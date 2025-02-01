//go:build unix

package aio

import (
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
	switch sotype {
	case syscall.SOCK_STREAM, syscall.SOCK_SEQPACKET:
		if setOptErr := setDefaultListenerSocketOpts(sock); setOptErr != nil {
			_ = syscall.Close(sock)
			err = setOptErr
			return
		}
		if setDeferAcceptErr := setDeferAccept(sock); setDeferAcceptErr != nil {
			_ = syscall.Close(sock)
			err = setDeferAcceptErr
			return
		}
		// bind
		sa := AddrToSockaddr(addr)
		bindErr := syscall.Bind(sock, sa)
		if bindErr != nil {
			_ = syscall.Close(sock)
			err = os.NewSyscallError("bind", bindErr)
			return
		}
		// listen
		listenErr := syscall.Listen(sock, somaxconn)
		if listenErr != nil {
			_ = syscall.Close(sock)
			err = os.NewSyscallError("listen", listenErr)
			return
		}
		// lsa
		lsa, getLsaErr := syscall.Getsockname(sock)
		if getLsaErr != nil {
			_ = syscall.Close(sock)
			err = os.NewSyscallError("getsockname", getLsaErr)
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
					_ = syscall.Close(sock)
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
					if err = setIPv4MulticastInterface(sock, multicastInterface); err != nil {
						_ = syscall.Close(sock)
						return
					}
				}
				if err = setIPv4MulticastLoopback(sock, false); err != nil {
					_ = syscall.Close(sock)
					return
				}
				if err = joinIPv4Group(sock, multicastInterface, ip4); err != nil {
					_ = syscall.Close(sock)
					return
				}
			} else {
				if multicastInterface != nil {
					if err = setIPv6MulticastInterface(sock, multicastInterface); err != nil {
						_ = syscall.Close(sock)
						return
					}
				}
				if err = setIPv6MulticastLoopback(sock, false); err != nil {
					_ = syscall.Close(sock)
					return
				}
				if err = joinIPv6Group(sock, multicastInterface, gaddr.IP); err != nil {
					_ = syscall.Close(sock)
					return
				}
			}
		}
		// bind
		sa := AddrToSockaddr(addr)
		bindErr := syscall.Bind(sock, sa)
		if bindErr != nil {
			err = os.NewSyscallError("bind", bindErr)
			_ = syscall.Close(sock)
			return
		}
		break
	default:
		break
	}

	// fd
	v = newListener(sock, network, family, sotype, proto, ipv6only, addr)
	return
}
