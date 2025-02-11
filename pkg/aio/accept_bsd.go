//go:build dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"github.com/brickingsoft/errors"
	"os"
	"syscall"
)

func Accept(fd NetFd, cb OperationCallback) {
	op := acquireOperator(fd)
	op.callback = cb
	op.completion = completeAccept

	cylinder := fd.Cylinder().(*KqueueCylinder)
	if err := cylinder.prepareRead(fd.Fd(), op); err != nil {
		releaseOperator(op)
		err = errors.New(
			"accept failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpAccept),
			errors.WithWrap(err),
		)
		cb(Userdata{}, err)
	}
}

func completeAccept(result int, op *Operator, err error) {
	cb := op.callback
	fd := op.fd.(NetFd)
	releaseOperator(op)
	if err != nil {
		err = errors.New(
			"accept failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpAccept),
			errors.WithWrap(err),
		)
		cb(Userdata{}, err)
		return
	}
	if result == 0 {
		err = errors.New(
			"accept failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpAccept),
			errors.WithWrap(ErrBusy),
		)
		cb(Userdata{}, err)
		return
	}
	sock := 0
	var sa syscall.Sockaddr
	for {
		sock, sa, err = syscall.Accept4(fd.Fd(), syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC)
		if err != nil {
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EINTR) || errors.Is(err, syscall.ECONNABORTED) {
				continue
			}
			err = errors.New(
				"accept failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpAccept),
				errors.WithWrap(os.NewSyscallError("accept4", err)),
			)
			cb(Userdata{}, err)
			return
		}
		break
	}

	// get local addr
	lsa, lsaErr := syscall.Getsockname(sock)
	if lsaErr != nil {
		_ = syscall.Close(sock)
		err = errors.New(
			"accept failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpAccept),
			errors.WithWrap(os.NewSyscallError("getsockname", lsaErr)),
		)
		cb(Userdata{}, err)
		return
	}
	la := SockaddrToAddr(fd.Network(), lsa)
	// get remote addr
	ra := SockaddrToAddr(fd.Network(), sa)
	// cylinder
	cylinder := nextKqueueCylinder()
	// conn
	conn := newNetFd(cylinder, sock, fd.Network(), fd.Family(), fd.SocketType(), fd.Protocol(), fd.IPv6Only(), la, ra)
	cb(Userdata{Fd: conn}, nil)
	return
}
