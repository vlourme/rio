package sys

import (
	"context"
	"runtime"
	"syscall"
)

type ControlContextFn func(ctx context.Context, network string, address string, raw syscall.RawConn) error

type ControlFn func(network string, address string, raw syscall.RawConn) error

func NewRawConn(fd int) syscall.RawConn {
	return &RawConn{fd: fd}
}

type RawConn struct {
	fd int
}

func (c *RawConn) Control(f func(fd uintptr)) error {
	f(uintptr(c.fd))
	runtime.KeepAlive(c.fd)
	return nil
}

func (c *RawConn) Read(f func(fd uintptr) (done bool)) (err error) {
	for {
		if f(uintptr(c.fd)) {
			break
		}
	}
	runtime.KeepAlive(c.fd)
	return
}

func (c *RawConn) Write(f func(fd uintptr) (done bool)) (err error) {
	for {
		if f(uintptr(c.fd)) {
			break
		}
	}
	runtime.KeepAlive(c.fd)
	return
}
