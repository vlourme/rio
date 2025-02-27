package tls

import (
	"crypto/tls"
	"reflect"
	"unsafe"
)

type Config struct {
	tls.Config
}

func ConfigFrom(c *tls.Config) *Config {
	rv := reflect.NewAt(reflect.TypeOf(Config{}), unsafe.Pointer(c))
	return rv.Interface().(*Config)
}

func ConfigTo(c *Config) *tls.Config {
	rv := reflect.NewAt(reflect.TypeOf(tls.Config{}), unsafe.Pointer(c))
	return rv.Interface().(*tls.Config)
}
