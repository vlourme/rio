//go:build amd64 || arm64 || loong64 || mips64 || mips64le || ppc64 || ppc64le || riscv64 || s390x || wasm

package aio

import (
	"runtime"
	"unsafe"
)

const (
	addrBits        = 48
	tagBits         = 64 - addrBits + 3
	aixAddrBits     = 57
	aixTagBits      = 64 - aixAddrBits + 3
	riscv64AddrBits = 56
	riscv64TagBits  = 64 - riscv64AddrBits + 3
)

var taggedPointerBits = (isAix() * aixTagBits) + (isRiscv64() * riscv64TagBits) + ((1 - isAix()) * (1 - isRiscv64()) * tagBits)

func isAix() int {
	if runtime.GOOS == "aix" {
		return 1
	}
	return 0
}

func isRiscv64() int {
	if runtime.GOARCH == "riscv64" {
		return 1
	}
	return 0
}

func TaggedPointerPack(ptr unsafe.Pointer, tag uintptr) TaggedPointer {
	if runtime.GOOS == "aix" {
		if runtime.GOARCH != "ppc64" {
			panic("check this code for aix on non-ppc64")
		}
		return TaggedPointer(uint64(uintptr(ptr))<<(64-aixAddrBits) | uint64(tag&(1<<aixTagBits-1)))
	}
	if runtime.GOARCH == "riscv64" {
		return TaggedPointer(uint64(uintptr(ptr))<<(64-riscv64AddrBits) | uint64(tag&(1<<riscv64TagBits-1)))
	}
	return TaggedPointer(uint64(uintptr(ptr))<<(64-addrBits) | uint64(tag&(1<<tagBits-1)))
}

type TaggedPointer uint64

func (tp TaggedPointer) Pointer() unsafe.Pointer {
	if runtime.GOARCH == "amd64" {
		// amd64 systems can place the stack above the VA hole, so we need to sign extend
		// val before unpacking.
		return unsafe.Pointer(uintptr(int64(tp) >> tagBits << 3))
	}
	if runtime.GOOS == "aix" {
		return unsafe.Pointer(uintptr((tp >> aixTagBits << 3) | 0xa<<56))
	}
	if runtime.GOARCH == "riscv64" {
		return unsafe.Pointer(uintptr(tp >> riscv64TagBits << 3))
	}
	return unsafe.Pointer(uintptr(tp >> tagBits << 3))
}

func (tp TaggedPointer) Tag() uintptr {
	return uintptr(tp & (1<<taggedPointerBits - 1))
}
