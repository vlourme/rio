package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/ring"
	"github.com/brickingsoft/rio/pkg/sys"
	"github.com/brickingsoft/rxp"
	"io"
	"net"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
)

type connection struct {
	ctx          context.Context
	cancel       context.CancelFunc
	fd           *sys.Fd
	readTimeout  atomic.Int64
	writeTimeout atomic.Int64
	exec         rxp.Executors
	ring         *ring.Ring
}

func (conn *connection) Read(b []byte) (n int, err error) {
	if len(b) == 0 {
		return
	}
	r := conn.ring
	op := r.AcquireOperation()
	fd := conn.fd.Socket()
	op.PrepareReceive(fd, b)
	if timeout := conn.readTimeout.Load(); timeout > 0 {
		op.SetTimeout(time.Duration(timeout))
	}
	if pushErr := r.Push(op); pushErr != nil {
		op.Discard()
		r.ReleaseOperation(op)
		err = &net.OpError{Op: "read", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: pushErr}
		// todo make err
		return
	}
	ctx := conn.ctx
	n, err = op.Await(ctx)
	r.ReleaseOperation(op)
	if err != nil {
		err = &net.OpError{Op: "read", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err} // todo make err
		return
	}
	if n == 0 && conn.fd.ZeroReadIsEOF() {
		err = io.EOF
		return
	}
	return
}

func (conn *connection) Write(b []byte) (n int, err error) {
	if len(b) == 0 {
		return
	}
	r := conn.ring
	op := r.AcquireOperation()
	fd := conn.fd.Socket()
	op.PrepareSend(fd, b)
	if timeout := conn.writeTimeout.Load(); timeout > 0 {
		op.SetTimeout(time.Duration(timeout))
	}
	if pushErr := r.Push(op); pushErr != nil {
		op.Discard()
		r.ReleaseOperation(op)
		err = &net.OpError{Op: "write", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: pushErr}
		// todo make err
		return
	}
	ctx := conn.ctx
	n, err = op.Await(ctx)
	r.ReleaseOperation(op)
	if err != nil {
		err = &net.OpError{Op: "write", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err} // todo make err
		return
	}
	return
}

func (conn *connection) Close() error {
	defer conn.cancel()
	if err := conn.fd.Close(); err != nil {
		return &net.OpError{Op: "close", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *connection) LocalAddr() net.Addr {
	return conn.fd.LocalAddr()
}

func (conn *connection) RemoteAddr() net.Addr {
	return conn.fd.RemoteAddr()
}

func (conn *connection) SetDeadline(t time.Time) error {
	if t.IsZero() {
		conn.readTimeout.Store(0)
		conn.writeTimeout.Store(0)
		return nil
	}
	timeout := time.Until(t)
	if timeout < 0 {
		timeout = 0
	}
	conn.readTimeout.Store(int64(timeout))
	conn.writeTimeout.Store(int64(timeout))
	return nil
}

func (conn *connection) SetReadDeadline(t time.Time) error {
	if t.IsZero() {
		conn.readTimeout.Store(0)
		return nil
	}
	timeout := time.Until(t)
	if timeout < 0 {
		timeout = 0
	}
	conn.readTimeout.Store(int64(timeout))
	return nil
}

func (conn *connection) SetWriteDeadline(t time.Time) error {
	if t.IsZero() {
		conn.writeTimeout.Store(0)
		return nil
	}
	timeout := time.Until(t)
	if timeout < 0 {
		timeout = 0
	}
	conn.writeTimeout.Store(int64(timeout))
	return nil
}

func newRawConnection(fd *sys.Fd) syscall.RawConn {
	return &rawConnection{fd: fd}
}

type rawConnection struct {
	fd *sys.Fd
}

func (conn *rawConnection) Control(f func(fd uintptr)) error {
	fd := conn.fd.Socket()
	f(uintptr(fd))
	runtime.KeepAlive(conn.fd)
	return nil
}

func (conn *rawConnection) Read(f func(fd uintptr) (done bool)) (err error) {
	fd := conn.fd.Socket()
	for {
		if f(uintptr(fd)) {
			break
		}
	}
	runtime.KeepAlive(conn.fd)
	return
}

func (conn *rawConnection) Write(f func(fd uintptr) (done bool)) (err error) {
	fd := conn.fd.Socket()
	for {
		if f(uintptr(fd)) {
			break
		}
	}
	runtime.KeepAlive(conn.fd)
	return
}
