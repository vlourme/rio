//go:build windows

package aio

import (
	"github.com/brickingsoft/errors"
	"golang.org/x/sys/windows"
	"os"
	"syscall"
	"unsafe"
)

func Accept(fd NetFd, cb OperationCallback) {
	// conn
	sock, sockErr := newSocket(fd.Family(), fd.SocketType(), fd.Protocol(), fd.IPv6Only())
	if sockErr != nil {
		err := errors.New(
			"accept failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpAccept),
			errors.WithWrap(sockErr),
		)
		cb(Userdata{}, err)
		return
	}
	// op
	op := acquireOperator(fd)
	// set sock
	op.handle = sock
	// set callback
	op.callback = cb
	// set completion
	op.completion = completeAccept

	// overlapped
	overlapped := &op.overlapped

	// sa
	var rawsa [2]syscall.RawSockaddrAny
	lsan := uint32(unsafe.Sizeof(rawsa[1]))
	rsa := &rawsa[0]
	rsan := uint32(unsafe.Sizeof(rawsa[0]))

	// accept
	acceptErr := syscall.AcceptEx(
		syscall.Handle(fd.Fd()), syscall.Handle(sock),
		(*byte)(unsafe.Pointer(rsa)), 0,
		lsan+16, rsan+16,
		&op.n, overlapped,
	)
	if acceptErr != nil && !errors.Is(syscall.ERROR_IO_PENDING, acceptErr) {
		_ = syscall.Closesocket(syscall.Handle(sock))
		err := errors.New(
			"accept failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpAccept),
			errors.WithWrap(os.NewSyscallError("acceptex", acceptErr)),
		)
		cb(Userdata{}, err)
		releaseOperator(op)
		return
	}
	return
}

func completeAccept(_ int, op *Operator, err error) {
	cb := op.callback
	fd := op.fd
	// sock
	sock := syscall.Handle(op.handle)
	releaseOperator(op)
	// handle error
	if err != nil {
		_ = syscall.Closesocket(sock)
		err = errors.New(
			"accept failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpAccept),
			errors.WithWrap(os.NewSyscallError("acceptex", err)),
		)
		cb(Userdata{}, err)
		return
	}
	// ln
	ln, _ := fd.(NetFd)
	lnFd := syscall.Handle(ln.Fd())

	// set SO_UPDATE_ACCEPT_CONTEXT
	setAcceptSocketOptErr := syscall.Setsockopt(
		sock,
		windows.SOL_SOCKET, windows.SO_UPDATE_ACCEPT_CONTEXT,
		(*byte)(unsafe.Pointer(&lnFd)),
		int32(unsafe.Sizeof(lnFd)),
	)
	if setAcceptSocketOptErr != nil {
		_ = syscall.Closesocket(sock)
		err = errors.New(
			"accept failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpAccept),
			errors.WithWrap(os.NewSyscallError("setsockopt", setAcceptSocketOptErr)),
		)
		cb(Userdata{}, err)
		return
	}

	// get local addr
	lsa, lsaErr := syscall.Getsockname(sock)
	if lsaErr != nil {
		_ = syscall.Closesocket(sock)
		err = errors.New(
			"accept failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpAccept),
			errors.WithWrap(os.NewSyscallError("getsockname", lsaErr)),
		)
		cb(Userdata{}, err)
		return
	}
	la := SockaddrToAddr(ln.Network(), lsa)

	// get remote addr
	rsa, rsaErr := syscall.Getpeername(sock)
	if rsaErr != nil {
		_ = syscall.Closesocket(sock)
		err = errors.New(
			"accept failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpAccept),
			errors.WithWrap(os.NewSyscallError("getsockname", rsaErr)),
		)
		cb(Userdata{}, err)
		return
	}
	ra := SockaddrToAddr(ln.Network(), rsa)

	// create iocp
	iocpErr := createSubIoCompletionPort(windows.Handle(sock))
	if iocpErr != nil {
		_ = syscall.Closesocket(sock)
		err = errors.New(
			"accept failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpAccept),
			errors.WithWrap(iocpErr),
		)
		cb(Userdata{}, err)
		return
	}

	// cylinder
	cylinder := nextIOCPCylinder()
	//conn
	conn := newNetFd(cylinder, int(sock), ln.Network(), ln.Family(), ln.SocketType(), ln.Protocol(), ln.IPv6Only(), la, ra)

	// callback
	cb(Userdata{Fd: conn}, nil)
	return
}
