//go:build linux

package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/sys"
	"net"
	"os"
	"sync"
	"syscall"
)

func ListenUnix(network string, addr *net.UnixAddr) (*UnixListener, error) {
	config := ListenConfig{}
	ctx := context.Background()
	return config.ListenUnix(ctx, network, addr)
}

func (lc *ListenConfig) ListenUnix(ctx context.Context, network string, addr *net.UnixAddr) (*UnixListener, error) {
	return nil, nil
}

func ListenUnixgram(network string, addr *net.UnixAddr) (*UnixConn, error) {
	config := ListenConfig{}
	ctx := context.Background()
	return config.ListenUnixgram(ctx, network, addr)
}

func (lc *ListenConfig) ListenUnixgram(ctx context.Context, network string, addr *net.UnixAddr) (*UnixConn, error) {
	return nil, nil
}

func newUnixListener(network string, addr *net.UnixAddr) (fd *sys.Fd, err error) {
	family := syscall.AF_UNIX
	sotype := 0
	switch network {
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
		err = net.UnknownNetworkError(network)
		return
	}
	// fd
	sock, sockErr := sys.NewSocket(family, sotype, 0)
	if sockErr != nil {
		err = sockErr
		return
	}
	fd = sys.NewFd(network, sock, family, sotype)
	// bind
	if err = fd.Bind(addr); err != nil {
		_ = fd.Close()
		return
	}
	// listen
	if sotype != syscall.SOCK_DGRAM {
		backlog := sys.MaxListenerBacklog()
		if err = syscall.Listen(sock, backlog); err != nil {
			_ = fd.Close()
			err = os.NewSyscallError("listen", err)
			return
		}
	}
	// set socket addr
	if sn, getSockNameErr := syscall.Getsockname(sock); getSockNameErr == nil {
		if sockname := sys.SockaddrToAddr(network, sn); sockname != nil {
			fd.SetLocalAddr(sockname)
		} else {
			fd.SetLocalAddr(addr)
		}
	} else {
		fd.SetLocalAddr(addr)
	}
	return
}

type UnixListener struct {
	fd         *sys.Fd
	path       string // todo default fd.laddr.String(), unlink: true
	unlink     bool
	unlinkOnce sync.Once
}

func (ln *UnixListener) Accept() (net.Conn, error) {
	//TODO implement me
	panic("implement me")
}

func (ln *UnixListener) Close() error {
	ln.unlinkOnce.Do(func() {
		if ln.path[0] != '@' && ln.unlink {
			_ = syscall.Unlink(ln.path)
		}
	})
	// todo
	return nil
}

func (ln *UnixListener) Addr() net.Addr {
	//TODO implement me
	panic("implement me")
}

func (ln *UnixListener) SetUnlinkOnClose(unlink bool) {
	ln.unlink = unlink
}

func (ln *UnixListener) File() (f *os.File, err error) {
	f, err = ln.file()
	if err != nil {
		err = &net.OpError{Op: "file", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
	}
	return
}

func (ln *UnixListener) file() (*os.File, error) {
	ns, call, err := ln.fd.Dup()
	if err != nil {
		if call != "" {
			err = os.NewSyscallError(call, err)
		}
		return nil, err
	}
	// todo check ok
	f := os.NewFile(uintptr(ns), ln.fd.Name())
	return f, nil
}

type UnixConn struct {
	conn
}
