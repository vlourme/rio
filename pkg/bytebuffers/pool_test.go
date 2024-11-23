package bytebuffers_test

import (
	"bytes"
	"github.com/brickingsoft/rio/pkg/bytebuffers"
	"os"
	"testing"
)

func TestGet(t *testing.T) {
	buf := bytebuffers.Get()
	buf.Write(bytes.Repeat([]byte("1"), os.Getpagesize()))
	bytebuffers.Put(buf)
	buf = bytebuffers.Get()
	t.Log(buf.Cap())
	bytebuffers.Put(buf)
}
