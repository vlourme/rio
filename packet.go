package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
)

type PacketConnection interface {
	transport.PacketConnection
}

const (
	defaultOOBBufferSize = 1024
)

func newPacketConnection(ctx context.Context, fd aio.NetFd) (conn PacketConnection) {
	conn = &packetConnection{
		connection: newConnection(ctx, fd),
		oob:        transport.NewInboundBuffer(),
		oobn:       defaultOOBBufferSize,
	}
	return
}

type packetConnection struct {
	connection
	oob  transport.InboundBuffer
	oobn int
}

func (conn *packetConnection) ReadFrom() (future async.Future[transport.PacketInbound]) {
	b, allocateErr := conn.rb.Allocate(conn.rbs)
	if allocateErr != nil {
		future = async.FailedImmediately[transport.PacketInbound](conn.ctx, errors.Join(ErrAllocate, allocateErr))
		return
	}
	if conn.disconnected() {
		future = async.FailedImmediately[transport.PacketInbound](conn.ctx, ErrClosed)
		return
	}

	var promise async.Promise[transport.PacketInbound]
	var promiseErr error
	if conn.readTimeout > 0 {
		promise, promiseErr = async.Make[transport.PacketInbound](conn.ctx, async.WithTimeout(conn.readTimeout))
	} else {
		promise, promiseErr = async.Make[transport.PacketInbound](conn.ctx)
	}
	if promiseErr != nil {
		conn.rb.AllocatedWrote(0)
		future = async.FailedImmediately[transport.PacketInbound](conn.ctx, promiseErr)
		return
	}
	promise.SetErrInterceptor(conn.handleReadFromErrInterceptor)

	aio.RecvFrom(conn.fd, b, func(userdata aio.Userdata, err error) {
		n := userdata.N
		conn.rb.AllocatedWrote(n)
		if err != nil {
			promise.Fail(aio.NewOpErr(aio.OpReadFrom, conn.fd, err))
			return
		}

		addr := userdata.Addr
		inbound := transport.NewPacketInbound(conn.rb, addr, n)
		promise.Succeed(inbound)
		return
	})

	future = promise.Future()
	return
}

func (conn *packetConnection) WriteTo(b []byte, addr net.Addr) (future async.Future[int]) {
	if len(b) == 0 {
		future = async.FailedImmediately[int](conn.ctx, ErrEmptyBytes)
		return
	}
	if addr == nil {
		future = async.FailedImmediately[int](conn.ctx, ErrNilAddr)
		return
	}
	if conn.disconnected() {
		future = async.FailedImmediately[int](conn.ctx, ErrClosed)
		return
	}
	var promise async.Promise[int]
	var promiseErr error
	if conn.writeTimeout > 0 {
		promise, promiseErr = async.Make[int](conn.ctx, async.WithTimeout(conn.writeTimeout))
	} else {
		promise, promiseErr = async.Make[int](conn.ctx)
	}
	if promiseErr != nil {
		future = async.FailedImmediately[int](conn.ctx, promiseErr)
		return
	}
	promise.SetErrInterceptor(conn.handleWriteToErrInterceptor)

	aio.SendTo(conn.fd, b, addr, func(userdata aio.Userdata, err error) {
		if err != nil {
			err = aio.NewOpWithAddrErr(aio.OpWriteTo, conn.fd, addr, err)
		}
		n := userdata.N
		promise.Complete(n, err)
		return
	})

	future = promise.Future()
	return
}

func (conn *packetConnection) SetReadMsgOOBBufferSize(size int) {
	if size < 1 {
		return
	}
	conn.oobn = size
}

func (conn *packetConnection) ReadMsg() (future async.Future[transport.PacketMsgInbound]) {
	b, allocateErr := conn.rb.Allocate(conn.rbs)
	if allocateErr != nil {
		future = async.FailedImmediately[transport.PacketMsgInbound](conn.ctx, errors.Join(ErrAllocate, allocateErr))
		return
	}
	oob, allocateOOBErr := conn.oob.Allocate(conn.oobn)
	if allocateOOBErr != nil {
		conn.rb.AllocatedWrote(0)
		future = async.FailedImmediately[transport.PacketMsgInbound](conn.ctx, errors.Join(ErrAllocate, allocateOOBErr))
		return
	}
	if conn.disconnected() {
		future = async.FailedImmediately[transport.PacketMsgInbound](conn.ctx, ErrClosed)
		return
	}

	var promise async.Promise[transport.PacketMsgInbound]
	var promiseErr error
	if conn.readTimeout > 0 {
		promise, promiseErr = async.Make[transport.PacketMsgInbound](conn.ctx, async.WithTimeout(conn.readTimeout))
	} else {
		promise, promiseErr = async.Make[transport.PacketMsgInbound](conn.ctx)
	}
	if promiseErr != nil {
		conn.rb.AllocatedWrote(0)
		conn.oob.AllocatedWrote(0)
		future = async.FailedImmediately[transport.PacketMsgInbound](conn.ctx, promiseErr)
		return
	}
	promise.SetErrInterceptor(conn.handleReadMsgErrInterceptor)

	aio.RecvMsg(conn.fd, b, oob, func(userdata aio.Userdata, err error) {
		if err != nil {
			conn.rb.AllocatedWrote(0)
			conn.oob.AllocatedWrote(0)
			promise.Fail(aio.NewOpErr(aio.OpReadMsg, conn.fd, err))
			return
		}
		n := userdata.N
		conn.rb.AllocatedWrote(n)

		oobn := userdata.OOBN
		conn.oob.AllocatedWrote(oobn)

		addr := userdata.Addr
		flags := userdata.MessageFlags
		inbound := transport.NewPacketMsgInbound(conn.rb, conn.oob, addr, n, oobn, flags)
		promise.Succeed(inbound)
		return
	})

	future = promise.Future()
	return
}

func (conn *packetConnection) WriteMsg(b []byte, oob []byte, addr net.Addr) (future async.Future[transport.PacketMsgOutbound]) {
	if len(b) == 0 {
		future = async.FailedImmediately[transport.PacketMsgOutbound](conn.ctx, ErrEmptyBytes)
		return
	}
	if addr == nil {
		future = async.FailedImmediately[transport.PacketMsgOutbound](conn.ctx, ErrNilAddr)
		return
	}
	if conn.disconnected() {
		future = async.FailedImmediately[transport.PacketMsgOutbound](conn.ctx, ErrClosed)
		return
	}

	var promise async.Promise[transport.PacketMsgOutbound]
	var promiseErr error
	if conn.writeTimeout > 0 {
		promise, promiseErr = async.Make[transport.PacketMsgOutbound](conn.ctx, async.WithTimeout(conn.writeTimeout))
	} else {
		promise, promiseErr = async.Make[transport.PacketMsgOutbound](conn.ctx)
	}
	if promiseErr != nil {
		future = async.FailedImmediately[transport.PacketMsgOutbound](conn.ctx, promiseErr)
		return
	}
	promise.SetErrInterceptor(conn.handleWriteMsgErrInterceptor)

	aio.SendMsg(conn.fd, b, oob, addr, func(userdata aio.Userdata, err error) {
		n := userdata.N
		oobn := userdata.OOBN
		if err != nil {
			err = aio.NewOpWithAddrErr(aio.OpWriteMsg, conn.fd, addr, err)
			if n == 0 {
				promise.Fail(err)
			} else {
				outbound := transport.NewPacketMsgOutbound(n, oobn, err)
				promise.Succeed(outbound)
			}
			return
		}
		outbound := transport.NewPacketMsgOutbound(n, oobn, nil)
		promise.Succeed(outbound)
		return
	})

	future = promise.Future()
	return
}

func (conn *packetConnection) Close() (future async.Future[async.Void]) {
	promise, promiseErr := async.Make[async.Void](conn.ctx, async.WithUnlimitedMode())
	if promiseErr != nil {
		aio.CloseImmediately(conn.fd)
		conn.rb.Close()
		conn.oob.Close()
		future = async.SucceedImmediately[async.Void](conn.ctx, async.Void{})
		return
	}
	conn.connection.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
		if cause != nil {
			promise.Fail(cause)
		} else {
			promise.Succeed(async.Void{})
		}
		conn.oob.Close()
	})
	future = promise.Future()
	return
}

func (conn *packetConnection) handleReadFromErrInterceptor(ctx context.Context, inbound transport.PacketInbound, err error) (future async.Future[transport.PacketInbound]) {
	if IsDeadlineExceeded(err) || IsUnexpectedContextFailed(err) {
		aio.Cancel(conn.fd.ReadOperator())
	} else if IsShutdown(err) {
		aio.CloseImmediately(conn.fd)
	}
	future = async.Immediately[transport.PacketInbound](ctx, inbound, err)
	return
}

func (conn *packetConnection) handleWriteToErrInterceptor(ctx context.Context, n int, err error) (future async.Future[int]) {
	if IsDeadlineExceeded(err) || IsUnexpectedContextFailed(err) {
		aio.Cancel(conn.fd.WriteOperator())
	} else if IsShutdown(err) {
		aio.CloseImmediately(conn.fd)
	}
	future = async.Immediately[int](ctx, n, err)
	return
}

func (conn *packetConnection) handleReadMsgErrInterceptor(ctx context.Context, inbound transport.PacketMsgInbound, err error) (future async.Future[transport.PacketMsgInbound]) {
	if IsDeadlineExceeded(err) || IsUnexpectedContextFailed(err) {
		aio.Cancel(conn.fd.ReadOperator())
	} else if IsShutdown(err) {
		aio.CloseImmediately(conn.fd)
	}
	future = async.Immediately[transport.PacketMsgInbound](ctx, inbound, err)
	return
}

func (conn *packetConnection) handleWriteMsgErrInterceptor(ctx context.Context, outbound transport.PacketMsgOutbound, err error) (future async.Future[transport.PacketMsgOutbound]) {
	if IsDeadlineExceeded(err) || IsUnexpectedContextFailed(err) {
		aio.Cancel(conn.fd.WriteOperator())
	} else if IsShutdown(err) {
		aio.CloseImmediately(conn.fd)
	}
	future = async.Immediately[transport.PacketMsgOutbound](ctx, outbound, err)
	return
}
