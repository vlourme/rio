//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"github.com/brickingsoft/errors"
	"net"
	"runtime"
	"syscall"
)

func Send(fd NetFd, b []byte, cb OperationCallback) {
	bLen := len(b)
	if bLen == 0 {
		cb(Userdata{}, nil)
		return
	}
	// op
	op := acquireOperator(fd)
	if setOp := fd.SetWOP(op); !setOp {
		releaseOperator(op)
		err := errors.New(
			"send failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSend),
			errors.WithWrap(errors.From(ErrRepeatOperation)),
		)
		cb(Userdata{}, err)
		return
	}
	// msg
	op.b = b

	// cb
	op.callback = cb
	// completion
	op.completion = completeSend

	cylinder := fd.Cylinder().(*KqueueCylinder)
	if err := cylinder.prepareWrite(fd.Fd(), op); err != nil {
		fd.RemoveWOP()
		releaseOperator(op)
		err = errors.New(
			"send failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSend),
			errors.WithWrap(err),
		)
		cb(Userdata{}, err)
	}
	return
}

func completeSend(result int, op *Operator, err error) {
	cb := op.callback
	fd := op.fd
	b := op.b
	fd.RemoveWOP()
	releaseOperator(op)

	if err != nil {
		err = errors.New(
			"send failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSend),
			errors.WithWrap(err),
		)
		cb(Userdata{}, err)
		return
	}
	if result == 0 {
		cb(Userdata{}, nil)
		return
	}

	if len(b) > result {
		b = b[:result]
	}
	for {
		n, wErr := syscall.Write(fd.Fd(), b)
		if wErr != nil {
			n = 0
			if errors.Is(wErr, syscall.EINTR) || errors.Is(wErr, syscall.EAGAIN) {
				continue
			}
			err = errors.New(
				"send failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpSend),
				errors.WithWrap(wErr),
			)
			cb(Userdata{}, err)
			break
		}
		cb(Userdata{N: n}, nil)
		break
	}

	runtime.KeepAlive(b)
	return
}

func SendTo(fd NetFd, b []byte, addr net.Addr, cb OperationCallback) {
	// op
	op := acquireOperator(fd)
	if setOp := fd.SetWOP(op); !setOp {
		releaseOperator(op)
		err := errors.New(
			"send to failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendTo),
			errors.WithWrap(errors.From(ErrRepeatOperation)),
		)
		cb(Userdata{}, err)
		return
	}
	// msg
	op.b = b
	op.sa = AddrToSockaddr(addr)

	// cb
	op.callback = cb
	// completion
	op.completion = completeSendTo

	cylinder := fd.Cylinder().(*KqueueCylinder)
	if err := cylinder.prepareWrite(fd.Fd(), op); err != nil {
		fd.RemoveWOP()
		releaseOperator(op)
		err = errors.New(
			"send to failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendTo),
			errors.WithWrap(err),
		)
		cb(Userdata{}, err)
	}
	return
}

func completeSendTo(result int, op *Operator, err error) {
	cb := op.callback
	fd := op.fd
	b := op.b
	sa := op.sa
	fd.RemoveWOP()
	releaseOperator(op)

	if err != nil {
		err = errors.New(
			"send to failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendTo),
			errors.WithWrap(err),
		)
		cb(Userdata{}, err)
		return
	}
	if result == 0 {
		cb(Userdata{}, nil)
		return
	}

	bLen := len(b)
	if bLen > result {
		b = b[:result]
		bLen = result
	}
	flags := 0
	for {
		wErr := syscall.Sendto(fd.Fd(), b, flags, sa)
		if wErr != nil {
			if errors.Is(wErr, syscall.EINTR) || errors.Is(wErr, syscall.EAGAIN) {
				continue
			}
			err = errors.New(
				"send to failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpSendTo),
				errors.WithWrap(wErr),
			)
			cb(Userdata{}, err)
			break
		}
		cb(Userdata{N: bLen}, nil)
		break
	}
	runtime.KeepAlive(b)
	return
}

func SendMsg(fd NetFd, b []byte, oob []byte, addr net.Addr, cb OperationCallback) {
	// msg
	bLen := len(b)
	oobLen := len(oob)
	if bLen == 0 && oobLen == 0 {
		cb(Userdata{}, nil)
	}
	if bLen > 0 && oobLen > 0 {
		err := errors.New(
			"send message failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendMsg),
			errors.WithWrap(errors.Define("aio: can not send bytes with oob")),
		)
		cb(Userdata{}, err)
		return
	}

	// op
	op := acquireOperator(fd)
	if setOp := fd.SetWOP(op); !setOp {
		releaseOperator(op)
		err := errors.New(
			"send message failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendMsg),
			errors.WithWrap(errors.From(ErrRepeatOperation)),
		)
		cb(Userdata{}, err)
		return
	}

	op.b = b
	op.oob = oob
	op.sa = AddrToSockaddr(addr)

	// cb
	op.callback = cb
	// completion
	op.completion = completeSendMsg

	cylinder := fd.Cylinder().(*KqueueCylinder)
	if err := cylinder.prepareWrite(fd.Fd(), op); err != nil {
		fd.RemoveWOP()
		releaseOperator(op)
		err = errors.New(
			"send message failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendMsg),
			errors.WithWrap(err),
		)
		cb(Userdata{}, err)
	}
	return
}

func completeSendMsg(result int, op *Operator, err error) {
	cb := op.callback

	fd := op.fd
	b := op.b
	oob := op.oob
	sa := op.sa
	fd.RemoveWOP()
	releaseOperator(op)

	if err != nil {
		err = errors.New(
			"send message failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpSendMsg),
			errors.WithWrap(err),
		)
		cb(Userdata{}, err)
		return
	}
	if result == 0 {
		cb(Userdata{}, nil)
		return
	}

	bLen := len(b)
	if bLen > result {
		b = b[:result]
		bLen = result
	}

	sock := fd.Fd()
	flags := 0
	for {
		wErr := syscall.Sendmsg(sock, b, oob, sa, flags)
		if wErr != nil {
			if errors.Is(wErr, syscall.EINTR) || errors.Is(wErr, syscall.EAGAIN) {
				continue
			}
			err = errors.New(
				"send message failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpSendMsg),
				errors.WithWrap(wErr),
			)
			cb(Userdata{}, err)
			break
		}
		cb(Userdata{N: bLen}, nil)
		break
	}
	runtime.KeepAlive(b)
	return
}
