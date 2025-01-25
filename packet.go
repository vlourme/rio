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
	promise, promiseErr := async.Make[transport.PacketInbound](conn.ctx)
	if promiseErr != nil {
		_ = conn.rb.AllocatedWrote(0)
		if async.IsExecutorsClosed(promiseErr) {
			future = async.FailedImmediately[transport.PacketInbound](conn.ctx, ErrClosed)
			aio.CloseImmediately(conn.fd)
		} else {
			future = async.FailedImmediately[transport.PacketInbound](conn.ctx, promiseErr)
		}
		return
	}

	aio.RecvFrom(conn.fd, b, func(userdata aio.Userdata, err error) {
		if err != nil {
			_ = conn.rb.AllocatedWrote(0)
			promise.Fail(aio.NewOpErr(aio.OpReadFrom, conn.fd, err))
			return
		}
		if awErr := conn.rb.AllocatedWrote(userdata.QTY); awErr != nil {
			promise.Fail(errors.Join(ErrAllocateWritten, awErr))
			return
		}
		addr, addrErr := userdata.Msg.Addr()
		if addrErr != nil {
			promise.Fail(aio.NewOpErr(aio.OpReadFrom, conn.fd, addrErr))
			return
		}
		inbound := transport.NewPacketInbound(conn.rb, addr, userdata.QTY)
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
	promise, promiseErr := async.Make[int](conn.ctx)
	if promiseErr != nil {
		if async.IsExecutorsClosed(promiseErr) {
			future = async.FailedImmediately[int](conn.ctx, ErrClosed)
			aio.CloseImmediately(conn.fd)
		} else {
			future = async.FailedImmediately[int](conn.ctx, promiseErr)
		}
		return
	}

	aio.SendTo(conn.fd, b, addr, func(userdata aio.Userdata, err error) {
		if err != nil {
			err = aio.NewOpWithAddrErr(aio.OpWriteTo, conn.fd, addr, err)
		}
		promise.Complete(userdata.QTY, err)
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
		_ = conn.rb.AllocatedWrote(0)
		future = async.FailedImmediately[transport.PacketMsgInbound](conn.ctx, errors.Join(ErrAllocate, allocateOOBErr))
		return
	}
	if conn.disconnected() {
		future = async.FailedImmediately[transport.PacketMsgInbound](conn.ctx, ErrClosed)
		return
	}
	promise, promiseErr := async.Make[transport.PacketMsgInbound](conn.ctx)
	if promiseErr != nil {
		_ = conn.rb.AllocatedWrote(0)
		_ = conn.oob.AllocatedWrote(0)
		if async.IsExecutorsClosed(promiseErr) {
			future = async.FailedImmediately[transport.PacketMsgInbound](conn.ctx, ErrClosed)
			aio.CloseImmediately(conn.fd)
		} else {
			future = async.FailedImmediately[transport.PacketMsgInbound](conn.ctx, promiseErr)
		}
		return
	}

	aio.RecvMsg(conn.fd, b, oob, func(userdata aio.Userdata, err error) {
		if err != nil {
			_ = conn.rb.AllocatedWrote(0)
			_ = conn.oob.AllocatedWrote(0)
			promise.Fail(aio.NewOpErr(aio.OpReadMsg, conn.fd, err))
			return
		}
		if awErr := conn.rb.AllocatedWrote(userdata.QTY); awErr != nil {
			promise.Fail(errors.Join(ErrAllocateWritten, awErr))
			return
		}
		oobn := userdata.Msg.ControlLen()
		if awErr := conn.oob.AllocatedWrote(oobn); awErr != nil {
			promise.Fail(errors.Join(ErrAllocateWritten, awErr))
			return
		}
		addr, addrErr := userdata.Msg.Addr()
		if addrErr != nil {
			promise.Fail(aio.NewOpErr(aio.OpReadMsg, conn.fd, addrErr))
			return
		}
		flags := int(userdata.Msg.Flags())
		inbound := transport.NewPacketMsgInbound(conn.rb, conn.oob, addr, userdata.QTY, oobn, flags)
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
	promise, promiseErr := async.Make[transport.PacketMsgOutbound](conn.ctx)
	if promiseErr != nil {
		if async.IsExecutorsClosed(promiseErr) {
			future = async.FailedImmediately[transport.PacketMsgOutbound](conn.ctx, ErrClosed)
			aio.CloseImmediately(conn.fd)
		} else {
			future = async.FailedImmediately[transport.PacketMsgOutbound](conn.ctx, promiseErr)
		}
		return
	}

	aio.SendMsg(conn.fd, b, oob, addr, func(userdata aio.Userdata, err error) {
		oobn := userdata.Msg.ControlLen()
		if err != nil {
			err = aio.NewOpErr(aio.OpWriteMsg, conn.fd, err)
			if userdata.QTY == 0 {
				promise.Fail(err)
			} else {
				outbound := transport.NewPacketMsgOutbound(userdata.QTY, oobn, err)
				promise.Succeed(outbound)
			}
			return
		}
		outbound := transport.NewPacketMsgOutbound(userdata.QTY, oobn, nil)
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
