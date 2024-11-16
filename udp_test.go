package rio_test

import (
	"github.com/brickingsoft/rio"
	"testing"
)

func TestParseUDPAddr(t *testing.T) {
	addrs := []string{
		"127.0.0.1:9000",
		":9000",
		"[::]:9000",
	}
	for _, addr := range addrs {
		v, err := rio.ParseUDPAddr(addr)
		if err != nil {
			t.Fatalf("ParseUDPAddr err: %v", err)
		}
		t.Log(v, v.IP.String(), v.Port, v.Network())
	}
}
