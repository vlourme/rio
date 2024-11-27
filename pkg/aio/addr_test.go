package aio_test

import (
	"github.com/brickingsoft/rio/pkg/aio"
	"reflect"
	"testing"
)

func TestResolveAddr(t *testing.T) {
	t.Log(aio.ResolveAddr("tcp6", "localhost:8080"))
	t.Log(aio.ResolveAddr("tcp", ":http"))
	t.Log(aio.ResolveAddr("tcp6", "[::]:http"))
	t.Log(aio.ResolveAddr("tcp6", "[::]:https"))
	t.Log(aio.ResolveAddr("tcp", ":https"))
	t.Log(aio.ResolveAddr("ip:tcp", "127.0.0.1"))

}

func TestAddrToSockaddr(t *testing.T) {
	addr, _, _, addErr := aio.ResolveAddr("ip", "0.0.0.0")
	if addErr != nil {
		t.Fatal(addErr)
	}
	t.Log(addr, reflect.TypeOf(addr))
	sa := aio.AddrToSockaddr(addr)
	t.Log(sa, reflect.TypeOf(sa))
}

func TestParseIpProto(t *testing.T) {
	t.Log(aio.ParseIpProto("ip:tcp"))
}
