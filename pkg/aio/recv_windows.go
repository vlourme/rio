//go:build windows

package aio

import (
	"github.com/brickingsoft/errors"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"syscall"
	"unsafe"
)

func Recv(fd NetFd, b []byte, cb OperationCallback) {
	// op
	op := acquireOperator(fd)
	// msg
	bLen := len(b)
	if bLen > maxRW {
		b = b[:maxRW]
	}
	buf := syscall.WSABuf{
		Len: uint32(bLen),
		Buf: &b[0],
	}
	flags := uint32(0)

	// cb
	op.callback = cb
	// completion
	op.completion = completeRecv

	// overlapped
	overlapped := &op.overlapped

	// recv
	err := syscall.WSARecv(
		syscall.Handle(fd.Fd()),
		&buf, 1,
		&op.n, &flags,
		overlapped,
		nil,
	)
	if err != nil && !errors.Is(syscall.ERROR_IO_PENDING, err) {
		err = errors.New(
			"receive failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpRecv),
			errors.WithWrap(os.NewSyscallError("wsa_recv", err)),
		)
		cb(Userdata{}, err)
		releaseOperator(op)
		return
	}
	return
}

func completeRecv(result int, op *Operator, err error) {
	cb := op.callback
	fd := op.fd
	releaseOperator(op)
	if err != nil {
		err = errors.New(
			"receive failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpRecv),
			errors.WithWrap(os.NewSyscallError("wsa_recv", err)),
		)
		cb(Userdata{}, err)
		return
	}
	if result == 0 && fd.ZeroReadIsEOF() {
		cb(Userdata{}, io.EOF)
		return
	}
	cb(Userdata{N: result}, nil)
	return
}

func RecvFrom(fd NetFd, b []byte, cb OperationCallback) {
	// op
	op := acquireOperator(fd)
	// msg
	bLen := len(b)
	if bLen > maxRW {
		b = b[:maxRW]
	}
	buf := syscall.WSABuf{
		Len: uint32(bLen),
		Buf: &b[0],
	}
	rsa := syscall.RawSockaddrAny{}
	rsaLen := int32(unsafe.Sizeof(rsa))
	op.msg.Name = &rsa
	//op.rsa = &rsa

	flags := uint32(0)
	// cb
	op.callback = cb
	// completion
	op.completion = completeRecvFrom

	// overlapped
	overlapped := &op.overlapped

	// recv from
	err := syscall.WSARecvFrom(
		syscall.Handle(fd.Fd()),
		&buf, 1,
		&op.n, &flags,
		&rsa, &rsaLen,
		overlapped,
		nil,
	)
	if err != nil && !errors.Is(syscall.ERROR_IO_PENDING, err) {
		err = errors.New(
			"receive from failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpRecvFrom),
			errors.WithWrap(os.NewSyscallError("wsa_recvfrom", err)),
		)
		cb(Userdata{}, err)
		releaseOperator(op)
		return
	}
	return
}

func completeRecvFrom(result int, op *Operator, err error) {
	cb := op.callback
	rsa := op.msg.Name
	releaseOperator(op)
	if err != nil {
		err = errors.New(
			"receive from failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpRecvFrom),
			errors.WithWrap(os.NewSyscallError("wsa_recvfrom", err)),
		)
		cb(Userdata{}, err)
		return
	}
	addr, addrErr := RawToAddr(rsa)
	if addrErr != nil {
		err = errors.New(
			"receive from failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpRecvFrom),
			errors.WithWrap(addrErr),
		)
		cb(Userdata{}, err)
		return
	}
	cb(Userdata{N: result, Addr: addr}, nil)
	return
}

func RecvMsg(fd NetFd, b []byte, oob []byte, cb OperationCallback) {
	// op
	op := acquireOperator(fd)
	// msg
	bLen := len(b)
	if bLen > maxRW {
		b = b[:maxRW]
	}
	rsa := syscall.RawSockaddrAny{}
	rsaLen := int32(unsafe.Sizeof(rsa))
	op.msg.Name = &rsa
	op.msg.Namelen = rsaLen

	if bLen > 0 {
		op.msg.Buffers = &windows.WSABuf{
			Len: uint32(bLen),
			Buf: &b[0],
		}
		op.msg.BufferCount = 1
	}
	if oobLen := len(oob); oobLen > 0 {
		op.msg.Control.Len = uint32(oobLen)
		op.msg.Control.Buf = &oob[0]
		if bLen == 0 && fd.SocketType() != syscall.SOCK_DGRAM {
			var dummy byte
			op.msg.Buffers = &windows.WSABuf{
				Buf: &dummy,
				Len: uint32(1),
			}
			op.msg.BufferCount = 1
		}
	}
	if fd.Family() == syscall.AF_UNIX {
		op.msg.Flags = readMsgFlags
	}

	// cb
	op.callback = cb
	// completion
	op.completion = completeRecvMsg

	// overlapped
	overlapped := &op.overlapped
	wsaoverlapped := (*windows.Overlapped)(unsafe.Pointer(overlapped))

	// recv msg
	err := windows.WSARecvMsg(
		windows.Handle(fd.Fd()),
		&op.msg,
		&op.n,
		wsaoverlapped,
		nil,
	)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		err = errors.New(
			"receive message failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpRecvMsg),
			errors.WithWrap(os.NewSyscallError("wsa_recvmsg", err)),
		)
		cb(Userdata{}, err)
		releaseOperator(op)
		return
	}
	return
}

func completeRecvMsg(result int, op *Operator, err error) {
	cb := op.callback
	msg := op.msg
	releaseOperator(op)
	if err != nil {
		err = errors.New(
			"receive message failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpRecvMsg),
			errors.WithWrap(os.NewSyscallError("wsa_recvmsg", err)),
		)
		cb(Userdata{}, err)
		return
	}
	addr, addrErr := RawToAddr(msg.Name)
	if addrErr != nil {
		err = errors.New(
			"receive message failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpRecvMsg),
			errors.WithWrap(addrErr),
		)
		cb(Userdata{}, err)
		return
	}
	oobn := int(msg.Control.Len)
	flags := int(msg.Flags)
	cb(Userdata{N: result, OOBN: oobn, Addr: addr, MessageFlags: flags}, nil)
	return
}
