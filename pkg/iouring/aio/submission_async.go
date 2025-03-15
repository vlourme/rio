//go:build linux

package aio

import (
	"syscall"
	"time"
)

func (vortex *Vortex) ConnectAsync(fd int, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareConnect(fd, addr, addrLen)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) AcceptAsync(fd int, addr *syscall.RawSockaddrAny, addrLen int, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).PrepareAccept(fd, addr, addrLen)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) AcceptDirectAsync(fd int, addr *syscall.RawSockaddrAny, addrLen int, sqeFlags uint8, fileIndex uint32) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithFiledIndex(fileIndex).WithDirect(true).PrepareAccept(fd, addr, addrLen)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) AcceptMultishotAsync(fd int, addr *syscall.RawSockaddrAny, addrLen int, buffer int, sqeFlags uint8) Future {
	op := NewOperation(buffer)
	op.Hijack()
	op.WithSQEFlags(sqeFlags).PrepareAcceptMultishot(fd, addr, addrLen)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) AcceptMultishotDirectAsync(fd int, addr *syscall.RawSockaddrAny, addrLen int, buffer int, sqeFlags uint8) Future {
	op := NewOperation(buffer)
	op.Hijack()
	op.WithSQEFlags(sqeFlags).WithDirect(true).PrepareAcceptMultishot(fd, addr, addrLen)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) CloseAsync(fd int) Future {
	op := vortex.acquireOperation()
	op.PrepareClose(fd)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) CloseDirectAsync(fd int) Future {
	op := vortex.acquireOperation()
	op.WithDirect(true).PrepareClose(fd)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) ReadFixedAsync(fd int, buf *FixedBuffer, deadline time.Time, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareReadFixed(fd, buf)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) WriteFixedAsync(fd int, buf *FixedBuffer, deadline time.Time, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareWriteFixed(fd, buf)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) ReceiveAsync(fd int, b []byte, deadline time.Time, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareReceive(fd, b)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) SendAsync(fd int, b []byte, deadline time.Time, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareSend(fd, b)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) SendZCAsync(fd int, b []byte, deadline time.Time, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareSendZC(fd, b)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) ReceiveMsgAsync(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32, deadline time.Time, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareReceiveMsg(fd, b, oob, addr, addrLen, flags)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) SendMsgAsync(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32, deadline time.Time, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareSendMsg(fd, b, oob, addr, addrLen, flags)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) SendMsgZCAsync(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32, deadline time.Time, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareSendMsgZC(fd, b, oob, addr, addrLen, flags)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) SpliceAsync(fdIn int, offIn int64, fdOut int, offOut int64, nbytes uint32, flags uint32, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).PrepareSplice(fdIn, offIn, fdOut, offOut, nbytes, flags)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) TeeAsync(fdIn int, fdOut int, nbytes uint32, flags uint32, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).PrepareTee(fdIn, fdOut, nbytes, flags)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}
