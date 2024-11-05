package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/async"
	"github.com/brickingsoft/rio/pkg/bytebufferpool"
	"github.com/brickingsoft/rio/pkg/sockets"
	"net"
	"time"
)

type Inbound interface {
	Buffer() (buf bytebufferpool.Buffer)
	Bytes() (n int)
}

type inbound struct {
	buf bytebufferpool.Buffer
	n   int
}

func (in *inbound) Buffer() (buf bytebufferpool.Buffer) {
	buf = in.buf
	return
}

func (in *inbound) Bytes() (n int) {
	n = in.n
	return
}

type Outbound interface {
	Content() (p []byte)
	Wrote() (n int)
}

type outbound struct {
	p []byte
	n int
}

func (out *outbound) Content() (p []byte) {
	p = out.p
	return
}

func (out *outbound) Wrote() (n int) {
	n = out.n
	return
}

type Connection interface {
	Context() (ctx context.Context)
	LocalAddr() (addr net.Addr)
	RemoteAddr() (addr net.Addr)
	SetDeadline(t time.Time) (err error)
	SetReadDeadline(t time.Time) (err error)
	SetWriteDeadline(t time.Time) (err error)
	SetReadBufferSize(size int)
	Read() (future async.Future[Inbound])
	Write(p []byte) (future async.Future[Outbound])
	Close() (err error)
}

const (
	defaultRWTimeout      = 15 * time.Second
	defaultReadBufferSize = 1024
)

func newConnection(ctx context.Context, inner sockets.Connection) (conn Connection) {
	connCtx, cancel := context.WithCancel(ctx)
	conn = &connection{
		ctx:    connCtx,
		cancel: cancel,
		inner:  inner,
		rbs:    defaultReadBufferSize,
		rb:     bytebufferpool.Get(),
		rto:    defaultRWTimeout,
		wto:    defaultRWTimeout,
	}
	return
}

type connection struct {
	ctx    context.Context
	cancel context.CancelFunc
	inner  sockets.Connection
	rbs    int
	rb     bytebufferpool.Buffer
	rto    time.Duration
	wto    time.Duration
}

func (conn *connection) Context() (ctx context.Context) {
	ctx = conn.ctx
	return
}

func (conn *connection) LocalAddr() (addr net.Addr) {
	addr = conn.inner.LocalAddr()
	return
}

func (conn *connection) RemoteAddr() (addr net.Addr) {
	addr = conn.inner.RemoteAddr()
	return
}

func (conn *connection) SetDeadline(t time.Time) (err error) {
	timeout := time.Until(t)
	if timeout < 1 {
		err = errors.New("deadline too short")
		return
	}
	err = conn.inner.SetDeadline(t)
	if err != nil {
		return
	}
	conn.rto = timeout
	conn.wto = timeout
	return
}

func (conn *connection) SetReadDeadline(t time.Time) (err error) {
	timeout := time.Until(t)
	if timeout < 1 {
		err = errors.New("deadline too short")
		return
	}
	err = conn.inner.SetReadDeadline(t)
	if err != nil {
		return
	}
	conn.rto = timeout
	return
}

func (conn *connection) SetWriteDeadline(t time.Time) (err error) {
	timeout := time.Until(t)
	if timeout < 1 {
		err = errors.New("deadline too short")
		return
	}
	err = conn.inner.SetWriteDeadline(t)
	if err != nil {
		return
	}
	conn.wto = timeout
	return
}

func (conn *connection) SetReadBufferSize(size int) {
	if size < 1 {
		size = defaultReadBufferSize
	}
	conn.rbs = size
	return
}

func (conn *connection) Read() (future async.Future[Inbound]) {
	promise, ok := async.TryPromise[Inbound](conn.ctx)
	if !ok {
		future = async.FailedImmediately[Inbound](conn.ctx, ErrBusy)
		return
	}
	timeout := time.Now().Add(conn.rto)
	promise.SetDeadline(timeout)
	area := conn.rb.ApplyAreaForWrite(conn.rbs)
	p := area.Bytes()
	conn.inner.Read(p, func(n int, err error) {
		area.Finish()
		if err != nil {
			promise.Fail(err)
			return
		}
		promise.Succeed(&inbound{
			buf: conn.rb,
			n:   n,
		})
		return
	})
	future = promise.Future()
	return
}

func (conn *connection) Write(p []byte) (future async.Future[Outbound]) {
	if len(p) == 0 {
		future = async.FailedImmediately[Outbound](conn.ctx, ErrEmptyPacket)
		return
	}
	promise, ok := async.TryPromise[Outbound](conn.ctx)
	if !ok {
		future = async.FailedImmediately[Outbound](conn.ctx, ErrBusy)
		return
	}
	timeout := time.Now().Add(conn.wto)
	promise.SetDeadline(timeout)
	conn.inner.Write(p, func(n int, err error) {
		if err != nil {
			promise.Fail(err)
			return
		}
		promise.Succeed(&outbound{
			p: p,
			n: n,
		})
		return
	})
	future = promise.Future()
	return
}

func (conn *connection) Close() (err error) {
	conn.cancel()
	err = conn.inner.Close()
	bytebufferpool.Put(conn.rb)
	return
}
