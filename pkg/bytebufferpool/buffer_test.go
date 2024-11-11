package bytebufferpool_test

import (
	"github.com/brickingsoft/rio/pkg/bytebufferpool"
	"testing"
)

func TestBufferPool_Get(t *testing.T) {
	buf := bytebufferpool.Get()
	t.Log(buf.Cap(), buf.Len())
	t.Log(buf.WriteString("0123456789"))
	t.Log(buf.Len())
	p5 := buf.Peek(5)
	t.Log(string(p5))
	buf.Discard(5)
	t.Log(buf.Next(5))
	t.Log(buf.Len())
	bytebufferpool.Put(buf)
}

func TestBuffer_Allocate(t *testing.T) {
	buf := bytebufferpool.Get()
	_, _ = buf.WriteString("0123456789")
	p := buf.Allocate(5)
	copy(p, "abc")
	buf.AllocatedWrote(3)
	_, _ = buf.WriteString("012")
	t.Log(string(buf.Peek(100)))
	bytebufferpool.Put(buf)
}

func TestBuffer_Next(t *testing.T) {
	buf := bytebufferpool.Get()
	defer bytebufferpool.Put(buf)
	_, _ = buf.WriteString("0123456789")
	p1, _ := buf.Next(5)
	t.Log(string(p1))
	_, _ = buf.WriteString("abcde")
	t.Log(string(p1))
	p1, _ = buf.Next(5)
	t.Log(string(p1))
}

func TestBuffer_Read(t *testing.T) {
	buf := bytebufferpool.Get()
	defer bytebufferpool.Put(buf)
	_, _ = buf.WriteString("0123456789")
	p := make([]byte, 5)
	buf.Read(p)
	t.Log(string(p), string(buf.Peek(5)))
}
