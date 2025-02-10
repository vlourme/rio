package rio

import (
	"context"
	"github.com/brickingsoft/errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/pkg/bytebuffers"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
	"sync/atomic"
)

type PacketConnection interface {
	transport.PacketConnection
}

func newPacketConnection(ctx context.Context, fd aio.NetFd) (conn PacketConnection) {
	conn = &packetConnection{
		connection: connection{
			ctx:    ctx,
			fd:     fd,
			closed: &atomic.Bool{},
			rb:     bytebuffers.Acquire(),
			rbs:    defaultReadBufferSize,
		},
		oobn: 0,
	}
	return
}

type packetConnection struct {
	connection
	oobn int
}

func (conn *packetConnection) ReadFrom() (future async.Future[transport.PacketInbound]) {
	ctx := conn.ctx
	rb := conn.rb
	rbs := conn.rbs
	if conn.disconnected() {
		err := errors.From(
			ErrReadFrom,
			errors.WithWrap(ErrClosed),
		)
		future = async.FailedImmediately[transport.PacketInbound](ctx, err)
		return
	}
	b, allocateErr := rb.Allocate(rbs)
	if allocateErr != nil {
		err := errors.From(
			ErrReadFrom,
			errors.WithWrap(ErrAllocateBytes),
		)
		future = async.FailedImmediately[transport.PacketInbound](ctx, err)
		return
	}
	var promise async.Promise[transport.PacketInbound]
	var promiseErr error
	if timeout := conn.readTimeout; timeout > 0 {
		promise, promiseErr = async.Make[transport.PacketInbound](ctx, async.WithTimeout(timeout))
	} else {
		promise, promiseErr = async.Make[transport.PacketInbound](ctx)
	}
	if promiseErr != nil {
		rb.Allocated(0)
		err := errors.From(
			ErrReadFrom,
			errors.WithWrap(promiseErr),
		)
		future = async.FailedImmediately[transport.PacketInbound](ctx, err)
		return
	}
	promise.SetErrInterceptor(conn.readFromErrInterceptor)

	closed := conn.closed

	aio.RecvFrom(conn.fd, b, func(userdata aio.Userdata, err error) {
		n := userdata.N
		rb.Allocated(n)
		if err != nil {
			err = errors.From(
				ErrReadFrom,
				errors.WithWrap(err),
			)
			promise.Fail(err)
			if closed.Load() {
				bytebuffers.Release(rb)
			}
			return
		}
		addr := userdata.Addr
		inbound := &packetInbound{
			Buffer: rb,
			addr:   addr,
		}
		promise.Succeed(inbound)
		if closed.Load() {
			bytebuffers.Release(rb)
		}
		return
	})

	future = promise.Future()
	return
}

func (conn *packetConnection) readFromErrInterceptor(ctx context.Context, _ transport.PacketInbound, err error) (future async.Future[transport.PacketInbound]) {
	if !errors.Is(err, ErrReadFrom) {
		err = errors.From(
			ErrReadFrom,
			errors.WithWrap(err),
		)
	}
	future = async.Immediately[transport.PacketInbound](ctx, nil, err)
	return
}

func (conn *packetConnection) WriteTo(b []byte, addr net.Addr) (future async.Future[int]) {
	ctx := conn.ctx
	if len(b) == 0 {
		err := errors.From(
			ErrWriteTo,
			errors.WithWrap(ErrEmptyBytes),
		)
		future = async.FailedImmediately[int](ctx, err)
		return
	}
	if addr == nil {
		err := errors.From(
			ErrWriteTo,
			errors.WithWrap(ErrNilAddr),
		)
		future = async.FailedImmediately[int](ctx, err)
		return
	}
	if conn.disconnected() {
		err := errors.From(
			ErrWriteTo,
			errors.WithWrap(ErrClosed),
		)
		future = async.FailedImmediately[int](ctx, err)
		return
	}
	var promise async.Promise[int]
	var promiseErr error
	if timeout := conn.writeTimeout; timeout > 0 {
		promise, promiseErr = async.Make[int](ctx, async.WithTimeout(timeout))
	} else {
		promise, promiseErr = async.Make[int](ctx)
	}
	if promiseErr != nil {
		err := errors.From(
			ErrWriteTo,
			errors.WithWrap(promiseErr),
		)
		future = async.FailedImmediately[int](ctx, err)
		return
	}
	promise.SetErrInterceptor(conn.writeToErrInterceptor)

	aio.SendTo(conn.fd, b, addr, func(userdata aio.Userdata, err error) {
		if err != nil {
			err = errors.From(
				ErrWriteTo,
				errors.WithWrap(err),
			)
			promise.Fail(err)
			return
		}
		n := userdata.N
		promise.Succeed(n)
		return
	})

	future = promise.Future()
	return
}

func (conn *packetConnection) writeToErrInterceptor(ctx context.Context, n int, err error) (future async.Future[int]) {
	if !errors.Is(err, ErrWriteTo) {
		err = errors.From(
			ErrWriteTo,
			errors.WithWrap(err),
		)
	}
	future = async.Immediately[int](ctx, n, err)
	return
}

func (conn *packetConnection) SetReadMsgOOBBufferSize(size int) {
	if size < 0 {
		size = 0
	}
	conn.oobn = size
}

func (conn *packetConnection) ReadMsg() (future async.Future[transport.PacketMsgInbound]) {
	ctx := conn.ctx
	rb := conn.rb
	rbs := conn.rbs
	var oob []byte

	if conn.disconnected() {
		err := errors.From(
			ErrReadMsg,
			errors.WithWrap(ErrClosed),
		)
		future = async.FailedImmediately[transport.PacketMsgInbound](ctx, err)
		return
	}

	b, allocateErr := rb.Allocate(rbs)
	if allocateErr != nil {
		err := errors.From(
			ErrReadMsg,
			errors.WithWrap(ErrAllocateBytes),
		)
		future = async.FailedImmediately[transport.PacketMsgInbound](ctx, err)
		return
	}

	var promise async.Promise[transport.PacketMsgInbound]
	var promiseErr error
	if timeout := conn.readTimeout; timeout > 0 {
		promise, promiseErr = async.Make[transport.PacketMsgInbound](ctx, async.WithTimeout(timeout))
	} else {
		promise, promiseErr = async.Make[transport.PacketMsgInbound](ctx)
	}
	if promiseErr != nil {
		rb.Allocated(0)
		err := errors.From(
			ErrReadMsg,
			errors.WithWrap(promiseErr),
		)
		future = async.FailedImmediately[transport.PacketMsgInbound](conn.ctx, err)
		return
	}
	promise.SetErrInterceptor(conn.readMsgErrInterceptor)

	if conn.oobn > 0 {
		oob = make([]byte, conn.oobn)
	}

	closed := conn.closed

	aio.RecvMsg(conn.fd, b, oob, func(userdata aio.Userdata, err error) {
		n := userdata.N
		rb.Allocated(n)
		if err != nil {
			err = errors.From(
				ErrReadMsg,
				errors.WithWrap(err),
			)
			promise.Fail(err)
			if closed.Load() {
				bytebuffers.Release(rb)
			}
			return
		}
		if oob != nil {
			oob = oob[:userdata.OOBN]
		}
		inbound := &packetInbound{
			Buffer: rb,
			addr:   userdata.Addr,
			oob:    oob,
			flags:  userdata.MessageFlags,
		}
		promise.Succeed(inbound)
		if closed.Load() {
			bytebuffers.Release(rb)
		}
		return
	})

	future = promise.Future()
	return
}

func (conn *packetConnection) readMsgErrInterceptor(ctx context.Context, _ transport.PacketMsgInbound, err error) (future async.Future[transport.PacketMsgInbound]) {
	if !errors.Is(err, ErrReadMsg) {
		err = errors.From(
			ErrReadMsg,
			errors.WithWrap(err),
		)
	}
	future = async.Immediately[transport.PacketMsgInbound](ctx, nil, err)
	return
}

func (conn *packetConnection) WriteMsg(b []byte, oob []byte, addr net.Addr) (future async.Future[transport.PacketMsgOutbound]) {
	if len(b) == 0 && len(oob) == 0 {
		err := errors.From(
			ErrWriteMsg,
			errors.WithWrap(ErrEmptyBytes),
		)
		future = async.FailedImmediately[transport.PacketMsgOutbound](conn.ctx, err)
		return
	}

	if addr == nil {
		err := errors.From(
			ErrWriteMsg,
			errors.WithWrap(ErrNilAddr),
		)
		future = async.FailedImmediately[transport.PacketMsgOutbound](conn.ctx, err)
		return
	}
	if conn.disconnected() {
		err := errors.From(
			ErrWriteMsg,
			errors.WithWrap(ErrClosed),
		)
		future = async.FailedImmediately[transport.PacketMsgOutbound](conn.ctx, err)
		return
	}

	var promise async.Promise[transport.PacketMsgOutbound]
	var promiseErr error
	if timeout := conn.writeTimeout; timeout > 0 {
		promise, promiseErr = async.Make[transport.PacketMsgOutbound](conn.ctx, async.WithTimeout(timeout))
	} else {
		promise, promiseErr = async.Make[transport.PacketMsgOutbound](conn.ctx)
	}
	if promiseErr != nil {
		err := errors.From(
			ErrWriteMsg,
			errors.WithWrap(promiseErr),
		)
		future = async.FailedImmediately[transport.PacketMsgOutbound](conn.ctx, err)
		return
	}
	promise.SetErrInterceptor(conn.writeMsgErrInterceptor)

	aio.SendMsg(conn.fd, b, oob, addr, func(userdata aio.Userdata, err error) {
		if err != nil {
			err = errors.From(
				ErrWriteMsg,
				errors.WithWrap(err),
			)
			promise.Fail(err)
			return
		}
		outbound := transport.PacketMsgOutbound{
			N:    userdata.N,
			OOBN: userdata.OOBN,
		}
		promise.Succeed(outbound)
		return
	})

	future = promise.Future()
	return
}

func (conn *packetConnection) writeMsgErrInterceptor(ctx context.Context, _ transport.PacketMsgOutbound, err error) (future async.Future[transport.PacketMsgOutbound]) {
	if !errors.Is(err, ErrWriteMsg) {
		err = errors.From(
			ErrWriteMsg,
			errors.WithWrap(err),
		)
	}
	future = async.FailedImmediately[transport.PacketMsgOutbound](ctx, err)
	return
}

func (conn *packetConnection) Close() (err error) {
	err = conn.connection.Close()
	return
}

type packetInbound struct {
	bytebuffers.Buffer
	addr  net.Addr
	oob   []byte
	flags int
}

func (in *packetInbound) Addr() net.Addr {
	return in.addr
}

func (in *packetInbound) OOB() []byte {
	return in.oob
}

func (in *packetInbound) Flags() (n int) {
	return in.flags
}
