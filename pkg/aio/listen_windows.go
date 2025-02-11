//go:build windows

package aio

import (
	"github.com/brickingsoft/errors"
	"golang.org/x/sys/windows"
	"net"
	"os"
	"syscall"
)

func newListenerFd(network string, family int, sotype int, proto int, ipv6only bool, addr net.Addr, multicastInterface *net.Interface) (v *netFd, err error) {
	// create sock
	sock, sockErr := newSocket(family, sotype, proto, ipv6only)
	if sockErr != nil {
		err = errors.New(
			"listen failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpListen),
			errors.WithWrap(sockErr),
		)
		return
	}
	handle := syscall.Handle(sock)
	switch sotype {
	case syscall.SOCK_STREAM, syscall.SOCK_SEQPACKET:
		setOptErr := setDefaultListenerSocketOpts(sock)
		if setOptErr != nil {
			_ = syscall.Closesocket(handle)
			err = errors.New(
				"listen failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpListen),
				errors.WithWrap(setOptErr),
			)
			return
		}
		// bind
		sa := AddrToSockaddr(addr)
		bindErr := syscall.Bind(handle, sa)
		if bindErr != nil {
			_ = syscall.Closesocket(handle)
			err = errors.New(
				"listen failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpListen),
				errors.WithWrap(os.NewSyscallError("bind", bindErr)),
			)
			return
		}
		// listen
		listenErr := syscall.Listen(handle, syscall.SOMAXCONN)
		if listenErr != nil {
			_ = syscall.Closesocket(handle)
			err = errors.New(
				"listen failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpListen),
				errors.WithWrap(os.NewSyscallError("listen", listenErr)),
			)
			return
		}
		// lsa
		lsa, getLsaErr := syscall.Getsockname(handle)
		if getLsaErr != nil {
			_ = syscall.Closesocket(handle)
			err = errors.New(
				"listen failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpListen),
				errors.WithWrap(os.NewSyscallError("getsockname", getLsaErr)),
			)
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
					err = errors.New(
						"listen failed",
						errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
						errors.WithMeta(errMetaOpKey, errMetaOpListen),
						errors.WithWrap(setOptErr),
					)
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
						err = errors.New(
							"listen failed",
							errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
							errors.WithMeta(errMetaOpKey, errMetaOpListen),
							errors.WithWrap(err),
						)
						return
					}
				}
				if err = setIPv4MulticastLoopback(handle, false); err != nil {
					_ = syscall.Closesocket(handle)
					err = errors.New(
						"listen failed",
						errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
						errors.WithMeta(errMetaOpKey, errMetaOpListen),
						errors.WithWrap(err),
					)
					return
				}
				if err = joinIPv4Group(handle, multicastInterface, ip4); err != nil {
					_ = syscall.Closesocket(handle)
					err = errors.New(
						"listen failed",
						errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
						errors.WithMeta(errMetaOpKey, errMetaOpListen),
						errors.WithWrap(err),
					)
					return
				}
			} else {
				if multicastInterface != nil {
					if err = setIPv6MulticastInterface(handle, multicastInterface); err != nil {
						_ = syscall.Closesocket(handle)
						err = errors.New(
							"listen failed",
							errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
							errors.WithMeta(errMetaOpKey, errMetaOpListen),
							errors.WithWrap(err),
						)
						return
					}
				}
				if err = setIPv6MulticastLoopback(handle, false); err != nil {
					_ = syscall.Closesocket(handle)
					err = errors.New(
						"listen failed",
						errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
						errors.WithMeta(errMetaOpKey, errMetaOpListen),
						errors.WithWrap(err),
					)
					return
				}
				if err = joinIPv6Group(handle, multicastInterface, gaddr.IP); err != nil {
					_ = syscall.Closesocket(handle)
					err = errors.New(
						"listen failed",
						errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
						errors.WithMeta(errMetaOpKey, errMetaOpListen),
						errors.WithWrap(err),
					)
					return
				}
			}
		}
		// bind
		sa := AddrToSockaddr(addr)
		bindErr := syscall.Bind(handle, sa)
		if bindErr != nil {
			_ = syscall.Closesocket(handle)
			err = errors.New(
				"listen failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpListen),
				errors.WithWrap(os.NewSyscallError("bind", bindErr)),
			)
			return
		}
		break
	default:
		break
	}

	// create iocp
	createListenIOCPErr := createSubIoCompletionPort(windows.Handle(sock))
	if createListenIOCPErr != nil {
		_ = syscall.Closesocket(handle)
		err = errors.New(
			"listen failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpListen),
			errors.WithWrap(createListenIOCPErr),
		)
		return
	}
	// cylinder
	cylinder := nextIOCPCylinder()

	// fd
	v = newNetFd(cylinder, sock, network, family, sotype, proto, ipv6only, addr, nil)
	return
}
