package sys

import (
	"github.com/brickingsoft/errors"
	"net"
	"os"
	"syscall"
)

type ListenOptions struct {
	MultipathTCP bool
	FastOpen     int
}

func NewListener(network string, address string) (*Listener, error) {
	addr, family, ipv6only, addrErr := ResolveAddr(network, address)
	if addrErr != nil {
		return nil, errors.New("new listener failed", errors.WithWrap(addrErr))
	}
	return &Listener{
		network:  network,
		address:  address,
		family:   family,
		ipv6only: ipv6only,
		addr:     addr,
	}, nil
}

type Listener struct {
	network  string
	address  string
	family   int
	ipv6only bool
	addr     net.Addr
}

func (ln *Listener) Listen(options ListenOptions) (fd *Fd, err error) {
	switch ln.addr.(type) {
	case *net.TCPAddr:
		fd, err = ln.listenTCP(options)
		break
	case *net.UnixAddr:
		fd, err = ln.listenUnix(options)
		break
	case *net.UDPAddr:
		fd, err = ln.listenUdp(options)
	case *net.IPAddr:
		fd, err = ln.listenIp(options)
		break
	default:
		err = &net.AddrError{Err: "unexpected address type", Addr: ln.addr.String()}
		break
	}
	if err != nil {
		return
	}
	return
}

func (ln *Listener) listenTCP(options ListenOptions) (fd *Fd, err error) {
	// proto
	proto := syscall.IPPROTO_TCP
	if options.MultipathTCP {
		if mp, ok := tryGetMultipathTCPProto(); ok {
			proto = mp
		}
	}
	// fd
	sock, sockErr := NewSocket(ln.family, syscall.SOCK_STREAM, proto)
	if sockErr != nil {
		err = sockErr
		return
	}
	fd = NewFd(ln.network, sock, ln.family, syscall.SOCK_STREAM)
	// ipv6
	if ln.ipv6only {
		if err = fd.SetIpv6only(true); err != nil {
			_ = fd.Close()
			return
		}
	}
	// reuse addr
	if err = fd.AllowReuseAddr(); err != nil {
		_ = fd.Close()
		return
	}
	// defer accept
	if err = syscall.SetsockoptInt(fd.sock, syscall.IPPROTO_TCP, syscall.TCP_DEFER_ACCEPT, 1); err != nil {
		_ = fd.Close()
		err = os.NewSyscallError("setsockopt", err)
		return
	}
	// bind
	if err = fd.Bind(ln.addr); err != nil {
		_ = fd.Close()
		return
	}
	// listen
	backlog := maxListenerBacklog()
	if err = syscall.Listen(fd.sock, backlog); err != nil {
		_ = fd.Close()
		err = os.NewSyscallError("listen", err)
		return
	}
	// set socket addr
	if sn, getSockNameErr := syscall.Getsockname(fd.sock); getSockNameErr == nil {
		addr := SockaddrToAddr(ln.network, sn)
		fd.SetLocalAddr(addr)
	} else {
		fd.SetLocalAddr(ln.addr)
	}

	return
}

func (ln *Listener) listenUnix(_ ListenOptions) (fd *Fd, err error) {
	sotype := 0
	switch ln.network {
	case "unix":
		sotype = syscall.SOCK_STREAM
		break
	case "unixpacket":
		sotype = syscall.SOCK_SEQPACKET
		break
	case "unixgram":
		sotype = syscall.SOCK_DGRAM
		break
	default:
		err = net.UnknownNetworkError(ln.network)
		return
	}
	// sock
	sock, sockErr := NewSocket(ln.family, sotype, 0)
	if sockErr != nil {
		err = sockErr
		return
	}
	fd = NewFd(ln.network, sock, ln.family, sotype)
	// fd
	// bind
	if err = fd.Bind(ln.addr); err != nil {
		_ = fd.Close()
		return
	}
	// listen
	if sotype != syscall.SOCK_DGRAM {
		backlog := maxListenerBacklog()
		if err = syscall.Listen(fd.sock, backlog); err != nil {
			_ = fd.Close()
			err = os.NewSyscallError("listen", err)
			return
		}
	}
	// set socket addr
	if sn, getSockNameErr := syscall.Getsockname(fd.sock); getSockNameErr == nil {
		addr := SockaddrToAddr(ln.network, sn)
		fd.SetLocalAddr(addr)
	} else {
		fd.SetLocalAddr(ln.addr)
	}
	return
}

func (ln *Listener) listenUdp(options ListenOptions) (fd *Fd, err error) {

	return
}

func (ln *Listener) listenIp(options ListenOptions) (fd *Fd, err error) {

	return
}
