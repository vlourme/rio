//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"net"
	"sync"
	"unsafe"
)

var (
	zerocopyPromiseAdaptors = sync.Pool{}
)

func acquireZerocopyAdaptor() *ZerocopyPromiseAdaptor {
	v := zerocopyPromiseAdaptors.Get()
	if v == nil {
		return &ZerocopyPromiseAdaptor{}
	}
	return v.(*ZerocopyPromiseAdaptor)
}

func releaseZerocopyAdaptor(adaptor *ZerocopyPromiseAdaptor) {
	if adaptor == nil {
		return
	}
	adaptor.n = 0
	zerocopyPromiseAdaptors.Put(adaptor)
}

type ZerocopyPromiseAdaptor struct {
	n int
}

func (adaptor *ZerocopyPromiseAdaptor) Handle(n int, flags uint32, err error) (bool, int, uint32, unsafe.Pointer, error) {
	if flags&liburing.IORING_CQE_F_MORE != 0 {
		adaptor.n = n
		return false, 0, 0, nil, nil
	}
	if flags&liburing.IORING_CQE_F_NOTIF != 0 {
		return true, adaptor.n, flags, nil, err
	}
	return true, n, flags, nil, err
}

func (c *Conn) Send(b []byte) (n int, err error) {
	if c.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}
	var (
		op      = AcquireOperationWithDeadline(c.writeDeadline)
		adaptor *ZerocopyPromiseAdaptor
	)
	if c.sendZCEnabled {
		adaptor = acquireZerocopyAdaptor()
		op.PrepareSendZC(c, b, adaptor)
	} else {
		op.PrepareSend(c, b)
	}
	n, _, err = c.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
	releaseZerocopyAdaptor(adaptor)
	if err != nil {
		return
	}
	return
}

func (c *Conn) SendTo(b []byte, addr net.Addr) (n int, err error) {
	sa, saErr := sys.AddrToSockaddr(addr)
	if saErr != nil {
		err = saErr
		return
	}
	rsa, rsaLen, rsaErr := sys.SockaddrToRawSockaddrAny(sa)
	if rsaErr != nil {
		err = rsaErr
		return
	}
	var (
		op      = AcquireOperationWithDeadline(c.writeDeadline)
		msg     = acquireMsg(b, nil, rsa, int(rsaLen), 0)
		adaptor *ZerocopyPromiseAdaptor
	)
	if c.sendMSGZCEnabled {
		op.PrepareSendMsgZC(c, msg, adaptor)
	} else {
		op.PrepareSendMsg(c, msg)
	}
	n, _, err = c.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
	releaseZerocopyAdaptor(adaptor)
	releaseMsg(msg)
	return
}

func (c *Conn) SendMsg(b []byte, oob []byte, addr net.Addr) (n int, oobn int, err error) {
	sa, saErr := sys.AddrToSockaddr(addr)
	if saErr != nil {
		err = saErr
		return
	}
	rsa, rsaLen, rsaErr := sys.SockaddrToRawSockaddrAny(sa)
	if rsaErr != nil {
		err = rsaErr
		return
	}
	var (
		op      = AcquireOperationWithDeadline(c.writeDeadline)
		msg     = acquireMsg(b, oob, rsa, int(rsaLen), 0)
		adaptor *ZerocopyPromiseAdaptor
	)

	if c.sendMSGZCEnabled {
		op.PrepareSendMsgZC(c, msg, adaptor)
	} else {
		op.PrepareSendMsg(c, msg)
	}
	n, _, err = c.eventLoop.SubmitAndWait(op)
	if err == nil {
		oobn = int(msg.Controllen)
	}
	ReleaseOperation(op)
	releaseZerocopyAdaptor(adaptor)
	releaseMsg(msg)
	return
}
