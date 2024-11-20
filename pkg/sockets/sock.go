package sockets

import (
	"errors"
	"golang.org/x/sys/windows"
	"net"
	"runtime"
	"time"
)

var (
	ErrEmptyPacket = errors.New("sockets: empty packet")
)

type ReadHandler func(n int, err error)
type WriteHandler func(n int, err error)
type CloseHandler func(err error)

type Connection interface {
	LocalAddr() (addr net.Addr)
	RemoteAddr() (addr net.Addr)
	SetReadTimeout(d time.Duration) (err error)
	SetWriteTimeout(d time.Duration) (err error)
	SetReadBuffer(n int) (err error)
	SetWriteBuffer(n int) (err error)
	Read(p []byte, handler ReadHandler)
	Write(p []byte, handler WriteHandler)
	Close(handler CloseHandler)
}

// *********************************************************************************************************************

type TCPConnection interface {
	Connection
	SetNoDelay(noDelay bool) (err error)
	SetLinger(sec int) (err error)
	SetKeepAlive(keepalive bool) (err error)
	SetKeepAlivePeriod(period time.Duration) (err error)
}

// *********************************************************************************************************************

type AcceptHandler func(conn Connection, err error)

type Listener interface {
	Addr() (addr net.Addr)
	Accept(handler AcceptHandler)
	Close(handler CloseHandler)
}

type UnixListener interface {
	Listener
	SetUnlinkOnClose(unlink bool)
}

// *********************************************************************************************************************

type ReadFromHandler func(n int, addr net.Addr, err error)
type ReadMsgHandler func(n int, oobn int, flags int, addr net.Addr, err error)
type WriteMsgHandler func(n int, oobn int, err error)

type PacketConnection interface {
	Connection
	ReadFrom(p []byte, handler ReadFromHandler)
	WriteTo(p []byte, addr net.Addr, handler WriteHandler)
	ReadMsg(p []byte, oob []byte, handler ReadMsgHandler)
	WriteMsg(p []byte, oob []byte, addr net.Addr, handler WriteMsgHandler)
}

// *********************************************************************************************************************

type DialHandler func(conn Connection, err error)

func Dial(network string, address string, opt Options, handler DialHandler) {
	raddr, family, ipv6only, addrErr := GetAddrAndFamily(network, address)
	if addrErr != nil {
		handler(nil, &net.OpError{Op: "dial", Net: network, Source: nil, Addr: nil, Err: addrErr})
		return
	}
	proto := 0
	switch network {
	case "tcp", "tcp4", "tcp6", "unix":
		if opt.MultipathTCP {
			proto = tryGetMultipathTCPProto()
		}
		var remoteAddr net.Addr
		switch ra := raddr.(type) {
		case *net.TCPAddr:
			if ra.IP == nil || ra.IP.IsUnspecified() {
				switch family {
				case windows.AF_INET:
					ra.IP = net.ParseIP("127.0.0.1")
					break
				case windows.AF_INET6:
					ra.IP = net.IPv6loopback
					break
				}
			}
			remoteAddr = ra
			break
		case *net.UnixAddr:
			if ra.Name == "" {
				handler(nil, &net.OpError{Op: "dial", Net: network, Source: nil, Addr: nil, Err: errors.New("invalid unix socket address")})
				return
			}
			break
		default:
			handler(nil, &net.OpError{Op: "dial", Net: network, Source: nil, Addr: nil, Err: errors.New("invalid socket address type")})
			return
		}
		connect(network, family, remoteAddr, ipv6only, proto, handler)
		break
	case "udp", "udp4", "udp6", "unixgram":
		laddr := opt.DialPacketConnLocalAddr
		var remoteAddr net.Addr
		switch ra := raddr.(type) {
		case *net.UDPAddr:
			if ra.IP == nil || ra.IP.IsUnspecified() {
				switch family {
				case windows.AF_INET:
					ra.IP = net.ParseIP("127.0.0.1")
					break
				case windows.AF_INET6:
					ra.IP = net.IPv6loopback
					break
				}
			}
			remoteAddr = ra
			break
		case *net.UnixAddr:
			if ra.Name == "" {
				handler(nil, &net.OpError{Op: "dial", Net: network, Source: nil, Addr: nil, Err: errors.New("invalid unix socket address")})
				return
			}
			if laddr != nil {
				_, isUnixAddr := laddr.(*net.UnixAddr)
				if !isUnixAddr {
					handler(nil, &net.OpError{Op: "dial", Net: network, Source: nil, Addr: nil, Err: errors.New("invalid local unix socket address")})
					return
				}
			}
			break
		default:
			handler(nil, &net.OpError{Op: "dial", Net: network, Source: nil, Addr: nil, Err: errors.New("invalid socket address type")})
			return
		}
		conn, err := newPacketConnection(network, family, windows.SOCK_DGRAM, laddr, remoteAddr, ipv6only, 0)
		if err != nil {
			handler(nil, &net.OpError{Op: "dial", Net: network, Source: nil, Addr: raddr, Err: err})
			return
		}
		handler(conn, nil)
		runtime.KeepAlive(conn)
		break
	case "ip", "ip4", "ip6":
		laddr := opt.DialPacketConnLocalAddr
		var remoteAddr net.Addr
		switch ra := raddr.(type) {
		case *net.IPAddr:
			if ra.IP == nil || ra.IP.IsUnspecified() {
				handler(nil, &net.OpError{Op: "dial", Net: network, Source: nil, Addr: nil, Err: errors.New("invalid ip socket address type")})
				return
			}
			remoteAddr = ra
		default:
			handler(nil, &net.OpError{Op: "dial", Net: network, Source: nil, Addr: nil, Err: errors.New("invalid socket address type")})
			return
		}
		conn, err := newPacketConnection(network, family, windows.SOCK_RAW, laddr, remoteAddr, ipv6only, 0)
		if err != nil {
			handler(nil, &net.OpError{Op: "dial", Net: network, Source: nil, Addr: raddr, Err: err})
			return
		}
		handler(conn, nil)
		runtime.KeepAlive(conn)
		break
	default:
		handler(nil, &net.OpError{Op: "dial", Net: network, Source: nil, Addr: raddr, Err: errors.New("invalid network")})
		break
	}
	return
}
