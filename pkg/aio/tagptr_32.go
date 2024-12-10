//go:build 386 || arm || mips || mipsle

package aio

import "unsafe"

const taggedPointerBits = 32

func TaggedPointerPack(ptr unsafe.Pointer, tag uintptr) TaggedPointer {
	return TaggedPointer(uintptr(ptr))<<taggedPointerBits | TaggedPointer(tag)
}

type TaggedPointer uint64

func (tp TaggedPointer) Pointer() unsafe.Pointer {
	return unsafe.Pointer(uintptr(tp >> taggedPointerBits))
}

func (tp TaggedPointer) Tag() uintptr {
	return uintptr(tp)
}
