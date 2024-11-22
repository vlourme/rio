package bytebufferpool_test

import (
	"github.com/brickingsoft/rio/pkg/bytebufferpool"
	"math"
	"os"
	"slices"
	"strings"
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

// BenchmarkBuffer_Read-20    	20722989	        60.08 ns/op	       0 B/op	       0 allocs/op
func BenchmarkBuffer_Read(b *testing.B) {
	buf := bytebufferpool.Get()
	defer bytebufferpool.Put(buf)
	pagesize := os.Getpagesize()
	firstData := []byte(strings.Repeat("abcd", pagesize/8))
	secondData := []byte(strings.Repeat("defg", pagesize/4))
	p := make([]byte, pagesize)
	_, _ = buf.Write(firstData)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = buf.Write(secondData)
		_, _ = buf.Read(p)
	}
}

func TestGrow(t *testing.T) {
	s := make([]byte, 8)
	t.Log(cap(slices.Grow(s, 64)))
}

func TestXXX(t *testing.T) {
	n := 0
	pagesize := os.Getpagesize()
	adjustedSize := int(math.Ceil(float64(n)/float64(pagesize)) * float64(pagesize))
	t.Log(adjustedSize, n-adjustedSize, n)
}
