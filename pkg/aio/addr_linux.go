//go:build linux

package aio

import (
	"errors"
	"syscall"
	"unsafe"
)

func SockaddrInet4ToRaw(sa *syscall.SockaddrInet4) (name *syscall.RawSockaddrAny, nameLen int32) {
	name = &syscall.RawSockaddrAny{}
	raw := (*syscall.RawSockaddrInet4)(unsafe.Pointer(name))
	raw.Family = syscall.AF_INET
	p := (*[2]byte)(unsafe.Pointer(&raw.Port))
	p[0] = byte(sa.Port >> 8)
	p[1] = byte(sa.Port)
	raw.Addr = sa.Addr
	nameLen = int32(unsafe.Sizeof(*raw))
	return
}

func SockaddrInet6ToRaw(sa *syscall.SockaddrInet6) (name *syscall.RawSockaddrAny, nameLen int32) {
	name = &syscall.RawSockaddrAny{}
	raw := (*syscall.RawSockaddrInet6)(unsafe.Pointer(name))
	raw.Family = syscall.AF_INET6
	p := (*[2]byte)(unsafe.Pointer(&raw.Port))
	p[0] = byte(sa.Port >> 8)
	p[1] = byte(sa.Port)
	raw.Scope_id = sa.ZoneId
	raw.Addr = sa.Addr
	nameLen = int32(unsafe.Sizeof(*raw))
	return
}

func SockaddrUnixToRaw(sa *syscall.SockaddrUnix) (name *syscall.RawSockaddrAny, nameLen int32) {
	name = &syscall.RawSockaddrAny{}
	raw := (*syscall.RawSockaddrUnix)(unsafe.Pointer(name))
	raw.Family = syscall.AF_UNIX
	path := make([]byte, len(sa.Name))
	copy(path, sa.Name)
	n := 0
	for n < len(path) && path[n] != 0 {
		n++
	}
	pp := []int8(unsafe.Slice((*int8)(unsafe.Pointer(&path[0])), n))
	copy(raw.Path[:], pp)
	nameLen = int32(unsafe.Sizeof(*raw))
	return
}

func SockaddrToRaw(sa syscall.Sockaddr) (name *syscall.RawSockaddrAny, nameLen int32, err error) {
	switch s := sa.(type) {
	case *syscall.SockaddrInet4:
		name, nameLen = SockaddrInet4ToRaw(s)
		return
	case *syscall.SockaddrInet6:
		name, nameLen = SockaddrInet6ToRaw(s)
		return
	case *syscall.SockaddrUnix:
		name, nameLen = SockaddrUnixToRaw(s)
		return
	default:
		err = errors.New("aio.SockaddrToRaw: invalid address type")
		return
	}
}
