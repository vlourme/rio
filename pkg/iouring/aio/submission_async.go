//go:build linux

package aio

import (
	"syscall"
	"time"
)

func (vortex *Vortex) ConnectAsync(fd int, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareConnect(fd, addr, addrLen)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}

func (vortex *Vortex) AcceptAsync(fd int, addr *syscall.RawSockaddrAny, addrLen int, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).PrepareAccept(fd, addr, addrLen)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}

func (vortex *Vortex) AcceptDirectAsync(fd int, addr *syscall.RawSockaddrAny, addrLen int, sqeFlags uint8, fileIndex uint32) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithFiledIndex(fileIndex).WithDirect(true).PrepareAccept(fd, addr, addrLen)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}

func (vortex *Vortex) AcceptMultishotAsync(fd int, addr *syscall.RawSockaddrAny, addrLen int, buffer int, sqeFlags uint8) Future {
	op := NewOperation(buffer)
	op.Hijack()
	op.WithSQEFlags(sqeFlags).PrepareAcceptMultishot(fd, addr, addrLen)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}

func (vortex *Vortex) AcceptMultishotDirectAsync(fd int, addr *syscall.RawSockaddrAny, addrLen int, buffer int, sqeFlags uint8) Future {
	op := NewOperation(buffer)
	op.Hijack()
	op.WithSQEFlags(sqeFlags).WithDirect(true).PrepareAcceptMultishot(fd, addr, addrLen)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}

func (vortex *Vortex) CloseAsync(fd int) Future {
	op := vortex.acquireOperation()
	op.PrepareClose(fd)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}

func (vortex *Vortex) CloseDirectAsync(fd int) Future {
	op := vortex.acquireOperation()
	op.WithDirect(true).PrepareClose(fd)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}

func (vortex *Vortex) ReadFixedAsync(fd int, buf *FixedBuffer, deadline time.Time, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareReadFixed(fd, buf)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}

func (vortex *Vortex) WriteFixedAsync(fd int, buf *FixedBuffer, deadline time.Time, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareWriteFixed(fd, buf)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}

func (vortex *Vortex) ReceiveAsync(fd int, b []byte, deadline time.Time, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareReceive(fd, b)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}

func (vortex *Vortex) SendAsync(fd int, b []byte, deadline time.Time, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareSend(fd, b)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}

func (vortex *Vortex) SendZCAsync(fd int, b []byte, deadline time.Time, sqeFlags uint8) Future {
	op := NewOperation(1)
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareSendZC(fd, b)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}

func (vortex *Vortex) ReceiveMsgAsync(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32, deadline time.Time, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareReceiveMsg(fd, b, oob, addr, addrLen, flags)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}

func (vortex *Vortex) SendMsgAsync(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32, deadline time.Time, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareSendMsg(fd, b, oob, addr, addrLen, flags)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}

func (vortex *Vortex) SendMsgZCAsync(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32, deadline time.Time, sqeFlags uint8) Future {
	op := NewOperation(1)
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareSendMsgZC(fd, b, oob, addr, addrLen, flags)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}

func (vortex *Vortex) SpliceAsync(fdIn int, offIn int64, fdOut int, offOut int64, nbytes uint32, flags uint32, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).PrepareSplice(fdIn, offIn, fdOut, offOut, nbytes, flags)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}

func (vortex *Vortex) TeeAsync(fdIn int, fdOut int, nbytes uint32, flags uint32, sqeFlags uint8) Future {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).PrepareTee(fdIn, fdOut, nbytes, flags)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}

func (vortex *Vortex) CancelAsync(target *Operation) Future {
	op := vortex.acquireOperation()
	op.PrepareCancel(target)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}

func (vortex *Vortex) FixedFdInstallAsync(fd int) Future {
	op := vortex.acquireOperation()
	op.PrepareFixedFdInstall(fd)
	var err error
	if submitted := vortex.submit(op); !submitted {
		err = ErrUncompleted
	}
	return Future{
		vortex: vortex,
		op:     op,
		err:    err,
	}
}
