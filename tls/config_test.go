package tls_test

import (
	stls "crypto/tls"
	"github.com/brickingsoft/rio/tls"
	"testing"
	"unsafe"
)

func TestConfigFrom(t *testing.T) {
	s := &stls.Config{}
	c := tls.ConfigFrom(s)
	ss := tls.ConfigTo(c)

	t.Log(unsafe.Pointer(s) == unsafe.Pointer(ss))
}
