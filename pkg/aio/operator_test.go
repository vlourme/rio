package aio_test

import (
	"github.com/brickingsoft/rio/pkg/aio"
	"testing"
	"unsafe"
)

func TestOperatorPtr(t *testing.T) {
	o1 := new(aio.Operator)
	p1 := uint64(uintptr(unsafe.Pointer(o1)))
	o2 := (*aio.Operator)(unsafe.Pointer(uintptr(p1)))
	t.Log(unsafe.Pointer(o1), unsafe.Pointer(o2), unsafe.Pointer(o1) == unsafe.Pointer(o2))
}
