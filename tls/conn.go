package tls

import (
	"crypto/tls"
	"reflect"
	"unsafe"
)

type Conn struct {
	tls.Conn
}

func ConnFrom(conn *tls.Conn) *Conn {
	rv := reflect.NewAt(reflect.TypeOf(Conn{}), unsafe.Pointer(conn))
	return rv.Interface().(*Conn)
}
