//go:build linux

package aio

import (
	"syscall"
	"time"
)

func (vortex *Vortex) PrepareOperation(op *Operation) Future {
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareConnect(fd int, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareConnect(fd, addr, addrLen)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareAccept(fd int, addr *syscall.RawSockaddrAny, addrLen int) Future {
	op := vortex.acquireOperation()
	op.PrepareAccept(fd, addr, addrLen)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareAcceptMultishot(fd int, addr *syscall.RawSockaddrAny, addrLen int, buffer int) Future {
	op := NewOperation(buffer)
	op.Hijack()
	op.PrepareAcceptMultishot(fd, addr, addrLen)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareClose(fd int) Future {
	op := vortex.acquireOperation()
	op.PrepareClose(fd)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareReadFixed(fd int, buf *FixedBuffer, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReadFixed(fd, buf)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareWriteFixed(fd int, buf *FixedBuffer, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareWriteFixed(fd, buf)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareReceive(fd int, b []byte, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReceive(fd, b)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareSend(fd int, b []byte, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSend(fd, b)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareSendZC(fd int, b []byte, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendZC(fd, b)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareReceiveMsg(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReceiveMsg(fd, b, oob, addr, addrLen, flags)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareSendMsg(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendMsg(fd, b, oob, addr, addrLen, flags)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareSendMsgZC(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32, deadline time.Time) Future {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendMsgZC(fd, b, oob, addr, addrLen, flags)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareSplice(fdIn int, offIn int64, fdOut int, offOut int64, nbytes uint32, flags uint32) Future {
	op := vortex.acquireOperation()
	op.PrepareSplice(fdIn, offIn, fdOut, offOut, nbytes, flags)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}

func (vortex *Vortex) PrepareTee(fdIn int, fdOut int, nbytes uint32, flags uint32) Future {
	op := vortex.acquireOperation()
	op.PrepareTee(fdIn, fdOut, nbytes, flags)
	vortex.submit(op)
	return Future{
		vortex: vortex,
		op:     op,
	}
}
