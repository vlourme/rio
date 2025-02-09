package bytebuffers_test

import (
	"bytes"
	"github.com/brickingsoft/rio/pkg/bytebuffers"
	"os"
	"testing"
)

func TestGet(t *testing.T) {
	buf := bytebuffers.Acquire()
	buf.Write(bytes.Repeat([]byte("1"), os.Getpagesize()))
	bytebuffers.Release(buf)
	buf = bytebuffers.Acquire()
	t.Log(buf.Cap())
	bytebuffers.Release(buf)
}

func BenchmarkCopy(b *testing.B) {
	s := []byte("hello world")
	for i := 0; i < b.N; i++ {
		p := make([]byte, len(s)*2)
		copy(p, s)
	}
	b.ReportAllocs()
	// BenchmarkCopy-20    	75599599	        14.18 ns/op	      24 B/op	       1 allocs/op
}

func BenchmarkAppend(b *testing.B) {
	s := []byte("hello world")
	for i := 0; i < b.N; i++ {
		p := make([]byte, len(s))
		p = append(p, s...)
	}
	b.ReportAllocs()
	// BenchmarkAppend-20    	41036864	        26.13 ns/op	      40 B/op	       2 allocs/op
}
