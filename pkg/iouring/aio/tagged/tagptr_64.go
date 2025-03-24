//go:build amd64 || arm64 || loong64 || mips64 || mips64le || ppc64 || ppc64le || riscv64 || s390x || wasm

package tagged

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

// PointerPack created a Pointer from a pointer and a tag.
// Tag bits that don't fit in the result are discarded.
func PointerPack[E any](ptr unsafe.Pointer, tag uintptr) Pointer[E] {
	if runtime.GOOS == "aix" {
		if runtime.GOARCH != "ppc64" {
			panic("check this code for aix on non-ppc64")
		}
		return Pointer[E](uint64(uintptr(ptr))<<(64-aixAddrBits) | uint64(tag&(1<<aixTagBits-1)))
	}
	if runtime.GOARCH == "riscv64" {
		return Pointer[E](uint64(uintptr(ptr))<<(64-riscv64AddrBits) | uint64(tag&(1<<riscv64TagBits-1)))
	}
	return Pointer[E](uint64(uintptr(ptr))<<(64-addrBits) | uint64(tag&(1<<tagBits-1)))
}

// Pointer returns the pointer from a Pointer.
func (tp Pointer[E]) Pointer() unsafe.Pointer {
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

// Tag returns the tag from a Pointer.
func (tp Pointer[E]) Tag() uintptr {
	isAix := 0
	if runtime.GOOS == "aix" {
		isAix = 1
	}
	isRiscv64 := 0
	if runtime.GOARCH == "riscv64" {
		isRiscv64 = 1
	}
	taggedPointerBits := (isAix * aixTagBits) + (isRiscv64 * riscv64TagBits) + ((1 - isAix) * (1 - isRiscv64) * tagBits)
	return uintptr(tp & (1<<taggedPointerBits - 1))
}

func (tp Pointer[E]) Value() *E {
	return (*E)(tp.Pointer())
}
