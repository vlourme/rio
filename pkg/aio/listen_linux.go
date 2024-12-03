//go:build linux

package aio

import (
	"bufio"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func newListenerFd(network string, family int, sotype int, proto int, addr net.Addr, multicastInterface *net.Interface) (v *netFd, err error) {
	// create sock
	sock, sockErr := newSocket(family, sotype, proto)
	if sockErr != nil {
		err = sockErr
		return
	}
	switch sotype {
	case syscall.SOCK_STREAM, syscall.SOCK_SEQPACKET:
		setOptErr := setDefaultListenerSocketOpts(sock)
		if setOptErr != nil {
			_ = syscall.Close(sock)
			err = setOptErr
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
		listenErr := syscall.Listen(sock, syscall.SOMAXCONN)
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
	case syscall.SOCK_RAW:
		// todo
		break
	default:
		break
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
	// op
	op := fd.ReadOperator()
	// ring
	cylinder := nextIOURingCylinder()
	entry := cylinder.ring.GetSQE()
	if entry == nil {
		cb(0, op.userdata, ErrBusy)
		return
	}

	// cb
	op.callback = cb
	// completion
	op.completion = completeAccept

	// ln
	lnFd := fd.Fd()
	// addr
	op.userdata.Msg.Name = new(syscall.RawSockaddrAny)
	op.userdata.Msg.Namelen = syscall.SizeofSockaddrAny
	addrPtr := uintptr(unsafe.Pointer(op.userdata.Msg.Name))
	addrLenPtr := uint64(uintptr(unsafe.Pointer(&op.userdata.Msg.Namelen)))

	// userdata
	userdata := uint64(uintptr(unsafe.Pointer(&op)))
	// prepare
	entry.prepareRW(opAccept, lnFd, addrPtr, 0, addrLenPtr, userdata, 0)
	return
}

func completeAccept(result int, op *Operator, err error) {
	// cb
	cb := op.callback
	// userdata
	userdata := op.userdata
	if err != nil {
		cb(result, userdata, os.NewSyscallError("io_uring_prep_accept", err))
		return
	}
	// conn
	connFd := result
	// ln
	ln, _ := op.fd.(NetFd)
	// addr
	// get local addr
	lsa, lsaErr := syscall.Getsockname(connFd)
	if lsaErr != nil {
		_ = syscall.Close(connFd)
		op.callback(result, userdata, os.NewSyscallError("getsockname", lsaErr))
		return
	}
	la := SockaddrToAddr(ln.Network(), lsa)

	// get remote addr
	ra, raErr := userdata.Msg.Addr()
	if raErr != nil {
		_ = syscall.Close(connFd)
		op.callback(result, userdata, errors.Join(errors.New("aio: get peername failed"), raErr))
		return
	}

	// conn
	conn := &netFd{
		handle:     connFd,
		network:    ln.Network(),
		family:     ln.Family(),
		socketType: ln.SocketType(),
		protocol:   ln.Protocol(),
		localAddr:  la,
		remoteAddr: ra,
		rop:        Operator{},
		wop:        Operator{},
	}
	conn.rop.fd = conn
	conn.wop.fd = conn

	userdata.Fd = conn
	// cb
	cb(connFd, userdata, nil)
	return
}

func SetReadBuffer(fd NetFd, n int) (err error) {
	handle := fd.Fd()
	err = syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, syscall.SO_RCVBUF, n)
	if err != nil {
		err = os.NewSyscallError("setsockopt", err)
		return
	}
	return
}

func SetWriteBuffer(fd NetFd, n int) (err error) {
	handle := fd.Fd()
	err = syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, syscall.SO_SNDBUF, n)
	if err != nil {
		err = os.NewSyscallError("setsockopt", err)
		return
	}
	return
}

func SetNoDelay(fd NetFd, noDelay bool) (err error) {
	handle := fd.Fd()
	err = syscall.SetsockoptInt(handle, syscall.IPPROTO_TCP, syscall.TCP_NODELAY, boolint(noDelay))
	if err != nil {
		err = os.NewSyscallError("setsockopt", err)
		return
	}
	return
}

func SetLinger(fd NetFd, sec int) (err error) {
	handle := fd.Fd()
	var l syscall.Linger
	if sec >= 0 {
		l.Onoff = 1
		l.Linger = int32(sec)
	} else {
		l.Onoff = 0
		l.Linger = 0
	}
	err = syscall.SetsockoptLinger(handle, syscall.SOL_SOCKET, syscall.SO_LINGER, &l)
	if err != nil {
		err = os.NewSyscallError("setsockopt", err)
		return
	}
	return
}

func SetKeepAlive(fd NetFd, keepalive bool) (err error) {
	handle := fd.Fd()
	err = syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, boolint(keepalive))
	if err != nil {
		err = os.NewSyscallError("setsockopt", err)
		return
	}
	return
}

func SetKeepAlivePeriod(fd NetFd, period time.Duration) (err error) {
	if period == 0 {
		period = defaultTCPKeepAliveIdle
	} else if period < 0 {
		return nil
	}
	secs := int(roundDurationUp(period, time.Second))
	handle := fd.Fd()
	err = syscall.SetsockoptInt(handle, syscall.IPPROTO_TCP, syscall.TCP_KEEPIDLE, secs)
	if err != nil {
		err = os.NewSyscallError("setsockopt", err)
		return
	}
	return
}

var (
	somaxconn = maxListenerBacklog()
)

const (
	maxUint16Value = 1<<16 - 1
)

func maxListenerBacklog() int {
	fd, err := os.Open("/proc/sys/net/core/somaxconn")
	if err != nil {
		return syscall.SOMAXCONN
	}
	defer fd.Close()

	rd := bufio.NewReader(fd)

	line, err := rd.ReadString('\n')
	if err != nil {
		return syscall.SOMAXCONN
	}

	f := strings.Fields(line)
	if len(f) < 1 {
		return syscall.SOMAXCONN
	}

	value, err := strconv.Atoi(f[0])
	if err != nil || value == 0 {
		return syscall.SOMAXCONN
	}

	if value > maxUint16Value {
		value = maxUint16Value
	}

	return value
}
