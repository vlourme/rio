package security_test

import (
	"crypto/rand"
	"crypto/tls"
	"github.com/brickingsoft/rio/security"
	"testing"
)

func TestFromConfig(t *testing.T) {
	tc := &tls.Config{NextProtos: []string{"h2", "http/1.1"}}
	key := [32]byte{}
	rand.Read(key[:])
	tc.SetSessionTicketKeys([][32]byte{key})
	c := security.FromConfig(tc)
	t.Log(c.NextProtos, len(c.SessionTicketKeys()))
	c.SetSessionTicketKeys([][32]byte{key})
	tc2 := security.ToConfig(c)
	t.Log(tc2.NextProtos)
	t.Log(tc == tc)
}
