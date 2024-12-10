package aio_test

import (
	"github.com/brickingsoft/rio/pkg/aio"
	"testing"
	"time"
	"unsafe"
)

func TestTaggedPointerPack(t *testing.T) {
	now := time.Now()
	tp := aio.TaggedPointerPack(unsafe.Pointer(&now), 1)
	ptr := tp.Pointer()
	n := (*time.Time)(ptr)
	t.Log(now)
	t.Log(*n)
	t.Log(unsafe.Pointer(&now), ptr, tp.Tag())
}
