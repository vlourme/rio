//go:build unix || windows

package sockets

import (
	"bytes"
	"errors"
	"net"
	"runtime"
	"syscall"
)

func interfaceToIPv4Addr(ifi *net.Interface) (net.IP, error) {
	if ifi == nil {
		return net.IPv4zero, nil
	}
	ifat, err := ifi.Addrs()
	if err != nil {
		return nil, err
	}
	for _, ifa := range ifat {
		switch v := ifa.(type) {
		case *net.IPAddr:
			if v.IP.To4() != nil {
				return v.IP, nil
			}
		case *net.IPNet:
			if v.IP.To4() != nil {
				return v.IP, nil
			}
		}
	}
	return nil, errors.New("no such network interface")
}

func setIPv4MulticastInterface(conn *connection, ifi *net.Interface) error {
	ip, err := interfaceToIPv4Addr(ifi)
	if err != nil {
		return wrapSyscallError("setsockopt", err)
	}
	var a [4]byte
	copy(a[:], ip.To4())
	err = syscall.SetsockoptInet4Addr(syscall.Handle(conn.fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_IF, a)
	runtime.KeepAlive(conn)
	return wrapSyscallError("setsockopt", err)
}

func setIPv4MulticastLoopback(conn *connection, v bool) error {
	err := syscall.SetsockoptInt(syscall.Handle(conn.fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_LOOP, boolint(v))
	runtime.KeepAlive(conn)
	return wrapSyscallError("setsockopt", err)
}

func joinIPv4Group(conn *connection, ifi *net.Interface, ip net.IP) error {
	mreq := &syscall.IPMreq{Multiaddr: [4]byte{ip[0], ip[1], ip[2], ip[3]}}
	if err := setIPv4MreqToInterface(mreq, ifi); err != nil {
		return err
	}
	err := syscall.SetsockoptIPMreq(syscall.Handle(conn.fd), syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, mreq)
	runtime.KeepAlive(conn)
	return wrapSyscallError("setsockopt", err)
}

func setIPv4MreqToInterface(mreq *syscall.IPMreq, ifi *net.Interface) error {
	if ifi == nil {
		return nil
	}
	ifat, err := ifi.Addrs()
	if err != nil {
		return err
	}
	for _, ifa := range ifat {
		switch v := ifa.(type) {
		case *net.IPAddr:
			if a := v.IP.To4(); a != nil {
				copy(mreq.Interface[:], a)
				goto done
			}
		case *net.IPNet:
			if a := v.IP.To4(); a != nil {
				copy(mreq.Interface[:], a)
				goto done
			}
		}
	}
done:
	if bytes.Equal(mreq.Multiaddr[:], net.IPv4zero.To4()) {
		return errors.New("no such multicast network interface")
	}
	return nil
}

func setIPv6MulticastInterface(conn *connection, ifi *net.Interface) error {
	var v int
	if ifi != nil {
		v = ifi.Index
	}
	err := syscall.SetsockoptInt(syscall.Handle(conn.fd), syscall.IPPROTO_IPV6, syscall.IPV6_MULTICAST_IF, v)
	runtime.KeepAlive(conn)
	return wrapSyscallError("setsockopt", err)
}

func setIPv6MulticastLoopback(conn *connection, v bool) error {
	err := syscall.SetsockoptInt(syscall.Handle(conn.fd), syscall.IPPROTO_IPV6, syscall.IPV6_MULTICAST_LOOP, boolint(v))
	runtime.KeepAlive(conn)
	return wrapSyscallError("setsockopt", err)
}

func joinIPv6Group(conn *connection, ifi *net.Interface, ip net.IP) error {
	mreq := &syscall.IPv6Mreq{}
	copy(mreq.Multiaddr[:], ip)
	if ifi != nil {
		mreq.Interface = uint32(ifi.Index)
	}
	err := syscall.SetsockoptIPv6Mreq(syscall.Handle(conn.fd), syscall.IPPROTO_IPV6, syscall.IPV6_JOIN_GROUP, mreq)
	runtime.KeepAlive(conn)
	return wrapSyscallError("setsockopt", err)
}
