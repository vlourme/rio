//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"net"
	"reflect"
	"syscall"
	"time"
)

func Connect(ctx context.Context, vortex *Vortex, deadline time.Time, network string, proto int, laddr net.Addr, raddr net.Addr, control sys.ControlContextFn) (fd *NetFd, err error) {
	// addr
	if laddr != nil && reflect.ValueOf(laddr).IsNil() {
		laddr = nil
	}
	if raddr != nil && reflect.ValueOf(raddr).IsNil() {
		raddr = nil
	}
	if laddr == nil && raddr == nil {
		err = errors.New("missing address")
		return
	}
	// network
	sotype := 0
	switch network {
	case "tcp", "tcp4", "tcp6":
		sotype = syscall.SOCK_STREAM
		break
	case "udp", "udp4", "udp6":
		sotype = syscall.SOCK_DGRAM
		break
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
		err = errors.New("unsupported network")
		return
	}
	// family
	family, ipv6only := sys.FavoriteAddrFamily(network, laddr, raddr, "dial")
	// sock
	var (
		regular = -1
		direct  = -1
	)
	if vortex.DirectAllocEnabled() {
		op := vortex.acquireOperation()
		op.WithDirectAlloc(true).PrepareSocket(family, sotype|syscall.SOCK_NONBLOCK, proto)
		direct, _, err = vortex.submitAndWait(op)
		vortex.releaseOperation(op)
	} else {
		regular, err = sys.NewSocket(family, sotype, proto)
	}
	if err != nil {
		return
	}
	// fd
	fd = &NetFd{
		Fd: Fd{
			regular:       regular,
			direct:        direct,
			isStream:      sotype == syscall.SOCK_STREAM,
			zeroReadIsEOF: sotype != syscall.SOCK_DGRAM && sotype != syscall.SOCK_RAW,
			vortex:        vortex,
		},
		family: family,
		sotype: sotype,
		net:    network,
		laddr:  laddr,
		raddr:  raddr,
	}
	// ipv6
	if ipv6only {
		if err = fd.SetIpv6only(true); err != nil {
			_ = fd.Close()
			return
		}
	}
	// zero copy
	if err = fd.SetZeroCopy(true); err != nil {
		_ = fd.Close()
		return
	}
	//  broadcast
	if err = fd.SetBroadcast(true); err != nil {
		_ = fd.Close()
		return
	}
	// control
	if control != nil {
		if regular == -1 {
			if regular, err = vortex.FixedFdInstall(direct); err == nil {
				_ = fd.Close()
				return
			}
			fd.regular = regular
		}
		raw, rawErr := fd.SyscallConn()
		if rawErr != nil {
			_ = fd.Close()
			err = rawErr
			return
		}
		var ctrlAddr string
		if raddr != nil {
			ctrlAddr = raddr.String()
		} else if laddr != nil {
			ctrlAddr = laddr.String()
		}
		if err = control(ctx, fd.CtrlNetwork(), ctrlAddr, raw); err != nil {
			_ = fd.Close()
			return
		}
	}
	// bind
	if laddr != nil {
		if err = fd.Bind(laddr); err != nil {
			_ = fd.Close()
			return
		}
	}
	// connect
	if raddr != nil {
		var (
			sa     syscall.Sockaddr
			rsa    *syscall.RawSockaddrAny
			rsaLen int32
		)

		if sa, err = sys.AddrToSockaddr(raddr); err != nil {
			return
		}
		if rsa, rsaLen, err = sys.SockaddrToRawSockaddrAny(sa); err != nil {
			return
		}
		op := fd.vortex.acquireOperation()
		op.WithDeadline(deadline).PrepareConnect(fd, rsa, int(rsaLen))
		_, _, err = fd.vortex.submitAndWait(op)
		fd.vortex.releaseOperation(op)
		if err != nil {
			_ = fd.Close()
			return
		}
	}
	return
}
