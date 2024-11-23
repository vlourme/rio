package iouring

import (
	"syscall"
	"unsafe"
)

// liburing: CMSG_ALIGN
func cmsgAlign(length uint64) uint64 {
	return (length + uint64(unsafe.Sizeof(uintptr(0))) - 1) & ^(uint64(unsafe.Sizeof(uintptr(0))) - 1)
}

// liburing: io_uring_recvmsg_out
type RecvmsgOut struct {
	Namelen    uint32
	ControlLen uint32
	PayloadLen uint32
	Flags      uint32
}

// liburing: io_uring_recvmsg_cmsg_nexthdr - https://manpages.debian.org/unstable/liburing-dev/io_uring_recvmsg_cmsg_nexthdr.3.en.html
func (o *RecvmsgOut) CmsgNexthdr(msgh *syscall.Msghdr, cmsg *syscall.Cmsghdr) *syscall.Cmsghdr {
	if cmsg.Len < syscall.SizeofCmsghdr {
		return nil
	}
	end := (*byte)(unsafe.Pointer(uintptr(unsafe.Pointer(o.CmsgFirsthdr(msgh))) + uintptr(o.ControlLen)))
	cmsg = (*syscall.Cmsghdr)(unsafe.Pointer(uintptr(unsafe.Pointer(cmsg)) + uintptr(cmsgAlign(cmsg.Len))))
	if uintptr(unsafe.Pointer(cmsg))+unsafe.Sizeof(*cmsg) > uintptr(unsafe.Pointer(end)) {
		return nil
	}
	if uintptr(unsafe.Pointer(cmsg))+uintptr(cmsgAlign(cmsg.Len)) > uintptr(unsafe.Pointer(end)) {
		return nil
	}

	return cmsg
}

// liburing: io_uring_recvmsg_name - https://manpages.debian.org/unstable/liburing-dev/io_uring_recvmsg_name.3.en.html
func (o *RecvmsgOut) Name() unsafe.Pointer {
	return unsafe.Pointer(uintptr(unsafe.Pointer(o)) + unsafe.Sizeof(*o))
}

// liburing: io_uring_recvmsg_cmsg_firsthdr - https://manpages.debian.org/unstable/liburing-dev/io_uring_recvmsg_cmsg_firsthdr.3.en.html
func (o *RecvmsgOut) CmsgFirsthdr(msgh *syscall.Msghdr) *syscall.Cmsghdr {
	if o.ControlLen < syscall.SizeofCmsghdr {
		return nil
	}

	return (*syscall.Cmsghdr)(unsafe.Pointer(uintptr(o.Name()) + uintptr(msgh.Namelen)))
}

// liburing: io_uring_recvmsg_payload - https://manpages.debian.org/unstable/liburing-dev/io_uring_recvmsg_payload.3.en.html
func (o *RecvmsgOut) Payload(msgh *syscall.Msghdr) unsafe.Pointer {
	return unsafe.Pointer(uintptr(unsafe.Pointer(o)) +
		unsafe.Sizeof(*o) +
		uintptr(msgh.Namelen) +
		uintptr(msgh.Controllen))
}

// liburing: io_uring_recvmsg_payload_length - https://manpages.debian.org/unstable/liburing-dev/io_uring_recvmsg_payload_length.3.en.html
func (o *RecvmsgOut) PayloadLength(bufLen int, msgh *syscall.Msghdr) uint32 {
	payloadStart := uintptr(o.Payload(msgh))
	payloadEnd := uintptr(unsafe.Pointer(o)) + uintptr(bufLen)

	return uint32(payloadEnd - payloadStart)
}

// liburing: io_uring_recvmsg_validate - https://manpages.debian.org/unstable/liburing-dev/io_uring_recvmsg_validate.3.en.html
func RecvmsgValidate(buf unsafe.Pointer, bufLen int, msgh *syscall.Msghdr) *RecvmsgOut {
	header := uintptr(msgh.Controllen) + uintptr(msgh.Namelen) + unsafe.Sizeof(RecvmsgOut{})
	if bufLen < 0 || uintptr(bufLen) < header {
		return nil
	}

	return (*RecvmsgOut)(buf)
}
