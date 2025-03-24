//go:build 386 || arm || mips || mipsle

package tagged

import "unsafe"

// The number of bits stored in the numeric tag of a Pointer
const taggedPointerBits = 32

// On 32-bit systems, Pointer has a 32-bit pointer and 32-bit count.

// PointerPack created a tagged Pointer from a pointer and a tag.
// Tag bits that don't fit in the result are discarded.
func PointerPack[E any](ptr unsafe.Pointer, tag uintptr) Pointer[E] {
	return Pointer[E](uintptr(ptr))<<32 | Pointer[E](tag)
}

// Pointer returns the pointer from a taggedPointer.
func (tp Pointer[E]) Pointer() unsafe.Pointer {
	return unsafe.Pointer(uintptr(tp >> 32))
}

// Tag returns the tag from a taggedPointer.
func (tp Pointer[E]) Tag() uintptr {
	return uintptr(tp)
}

func (tp Pointer[E]) Value() *E {
	return (*E)(tp.Pointer())
}
