package tagged_test

import (
	"github.com/brickingsoft/rio/pkg/iouring/aio/tagged"
	"math"
	"runtime"
	"testing"
	"time"
	"unsafe"
)

func TestTaggedPointerPack(t *testing.T) {
	n := func() *time.Time {
		v := time.Now()
		return &v
	}()
	tp1 := tagged.PointerPack[time.Time](unsafe.Pointer(n), math.MaxUint16)
	t.Log(tp1, tp1.Pointer(), tp1.Tag(), math.MaxUint16)
	tp2 := uintptr(tp1)
	tp1 = 0
	n = nil
	runtime.GC()
	np := tagged.Pointer[time.Time](tp2).Value()
	t.Log(*np)

}
