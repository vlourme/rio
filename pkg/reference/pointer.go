package reference

import (
	"io"
	"reflect"
	"sync/atomic"
)

func Make[E io.Closer](value E) *Pointer[E] {
	if reflect.ValueOf(value).IsNil() {
		panic("value is nil")
	}
	return &Pointer[E]{value: value}
}

type Pointer[E io.Closer] struct {
	value E
	count atomic.Int64
}

func (pointer *Pointer[E]) Value() E {
	pointer.count.Add(1)
	return pointer.value
}

func (pointer *Pointer[E]) Count() int64 {
	return pointer.count.Load()
}

func (pointer *Pointer[E]) Close() error {
	if n := pointer.count.Add(-1); n == 0 {
		return pointer.value.Close()
	}
	if pointer.count.Load() == -1 {
		return pointer.value.Close()
	}
	return nil
}
