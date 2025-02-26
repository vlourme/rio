//go:build linux

package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/sys"
	"net"
	"reflect"
	"syscall"
	"time"
)

func ListenIP(network string, addr *net.IPAddr) (*IPConn, error) {
	config := ListenConfig{}
	ctx := context.Background()
	return config.ListenIP(ctx, network, addr)
}

func (lc *ListenConfig) ListenIP(ctx context.Context, network string, addr *net.IPAddr) (*IPConn, error) {
	return nil, nil
}

type IPConn struct {
	conn
	useMsgZC bool
}

func (c *IPConn) ReadFromIP(b []byte) (n int, addr *net.IPAddr, err error) {
	if !c.ok() {
		return 0, nil, syscall.EINVAL
	}

	if len(b) == 0 {
		return 0, nil, &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}

	ctx := c.ctx
	fd := c.fd.Socket()
	vortex := c.vortex

	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	deadline := c.readDeadline

RETRY:
	future := vortex.PrepareReceiveMsg(ctx, fd, b, nil, rsa, rsaLen, 0, deadline)
	n, err = future.Await(ctx)
	if err != nil {
		if errors.Is(err, syscall.EBUSY) {
			if !deadline.IsZero() && deadline.Before(time.Now()) {
				err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: aio.Timeout}
				return
			}
			goto RETRY
		}
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}

	sa, saErr := sys.RawSockaddrAnyToSockaddr(rsa)
	if saErr != nil {
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: saErr}
		return
	}
	a := sys.SockaddrToAddr(c.fd.Net(), sa)
	ok := false
	addr, ok = a.(*net.IPAddr)
	if !ok {
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: errors.New("wrong address type")}
		return
	}

	return n, addr, err
}

func (c *IPConn) ReadFrom(b []byte) (int, net.Addr, error) {
	return c.ReadFromIP(b)
}

func (c *IPConn) ReadMsgIP(b, oob []byte) (n, oobn, flags int, addr *net.IPAddr, err error) {
	if !c.ok() {
		return 0, 0, 0, nil, syscall.EINVAL
	}
	bLen := len(b)
	oobLen := len(oob)
	if bLen == 0 && oobLen == 0 {
		return 0, 0, 0, nil, &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}

	ctx := c.ctx
	fd := c.fd.Socket()
	vortex := c.vortex

	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	deadline := c.readDeadline

RETRY:
	future := vortex.PrepareReceiveMsg(ctx, fd, b, oob, rsa, rsaLen, 0, deadline)
	rn, msg, rErr := future.AwaitMsg(ctx)
	if rErr != nil {
		if errors.Is(err, syscall.EBUSY) {
			if !deadline.IsZero() && deadline.Before(time.Now()) {
				err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: aio.Timeout}
				return
			}
			goto RETRY
		}
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: rErr}
		return
	}

	n = rn
	oobn = int(msg.Controllen)
	flags = int(msg.Flags)

	sa, saErr := sys.RawSockaddrAnyToSockaddr(rsa)
	if saErr != nil {
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: saErr}
		return
	}
	a := sys.SockaddrToAddr(c.fd.Net(), sa)
	ok := false
	addr, ok = a.(*net.IPAddr)
	if !ok {
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: net.InvalidAddrError("wrong address type")}
		return
	}
	return
}

func (c *IPConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	if len(b) == 0 || reflect.ValueOf(addr).IsNil() {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	ipAddr, isIpAddr := addr.(*net.IPAddr)
	if !isIpAddr {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	if ipAddr.Network() != c.fd.Net() {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	sa, saErr := sys.AddrToSockaddr(ipAddr)
	if saErr != nil {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: saErr}
	}
	return c.writeTo(b, sa)
}

func (c *IPConn) WriteToIP(b []byte, addr *net.IPAddr) (int, error) {
	return c.WriteTo(b, addr)
}

func (c *IPConn) writeTo(b []byte, addr syscall.Sockaddr) (n int, err error) {
	rsa, rsaLen, rsaErr := sys.SockaddrToRawSockaddrAny(addr)
	if rsaErr != nil {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: rsaErr}
	}

	ctx := c.ctx
	fd := c.fd.Socket()
	vortex := c.vortex

	deadline := c.writeDeadline

RETRY:
	if c.useMsgZC {
		future := vortex.PrepareSendMsgZC(ctx, fd, b, nil, rsa, int(rsaLen), 0, deadline)
		n, err = future.Await(ctx)
	} else {
		future := vortex.PrepareSendMsg(ctx, fd, b, nil, rsa, int(rsaLen), 0, deadline)
		n, err = future.Await(ctx)
	}

	if err != nil {
		if errors.Is(err, syscall.EBUSY) {
			if !deadline.IsZero() && deadline.Before(time.Now()) {
				err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: aio.Timeout}
				return
			}
			goto RETRY
		}
		err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}

	return
}

func (c *IPConn) WriteMsgIP(b, oob []byte, addr *net.IPAddr) (n, oobn int, err error) {
	if !c.ok() {
		return 0, 0, syscall.EINVAL
	}
	if len(b) == 0 && len(oob) == 0 {
		return 0, 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	if addr == nil {
		return 0, 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	if addr.Network() != c.fd.Net() {
		return 0, 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	sa, saErr := sys.AddrToSockaddr(addr)
	if saErr != nil {
		return 0, 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: saErr}
	}
	rsa, rsaLen, rsaErr := sys.SockaddrToRawSockaddrAny(sa)
	if rsaErr != nil {
		err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: rsaErr}
		return
	}

	if len(b) == 0 && c.fd.SocketType() != syscall.SOCK_DGRAM {
		b = []byte{0}
	}

	ctx := c.ctx
	fd := c.fd.Socket()
	vortex := c.vortex

	deadline := c.writeDeadline

RETRY:
	if c.useMsgZC {
		future := vortex.PrepareSendMsgZC(ctx, fd, b, oob, rsa, int(rsaLen), 0, deadline)
		wn, msg, wErr := future.AwaitMsg(ctx)
		if wErr == nil {
			oobn = int(msg.Controllen)
		}
		n, err = wn, wErr
	} else {
		future := vortex.PrepareSendMsg(ctx, fd, b, oob, rsa, int(rsaLen), 0, deadline)
		wn, msg, wErr := future.AwaitMsg(ctx)
		if wErr == nil {
			oobn = int(msg.Controllen)
		}
		n, err = wn, wErr
	}

	if err != nil {
		if errors.Is(err, syscall.EBUSY) {
			if !deadline.IsZero() && deadline.Before(time.Now()) {
				err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: aio.Timeout}
				return
			}
			goto RETRY
		}
		err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}
	return
}
