package bytebufferpool

import (
	"io"
	"math"
	"os"
	"slices"
	"unicode/utf8"
	"unsafe"
)

type Buffer interface {
	Len() (n int)
	Cap() (n int)
	Available() (n int)
	Peek(n int) (p []byte)
	Next(n int) (p []byte, err error)
	Discard(n int)
	Read(p []byte) (n int, err error)
	Write(p []byte) (n int, err error)
	WriteString(s string) (n int, err error)
	WriteByte(c byte) (err error)
	WriteRune(r rune) (n int, err error)
	ApplyAreaForWrite(n int) (area AreaOfBuffer)
	WritePending() bool
	Empty() bool
	Reset()
}

var (
	pageszie               = os.Getpagesize()
	oneQuarterOfPagesize   = pageszie / 4
	halfOfPagesize         = pageszie / 2
	threeQuarterOfPagesize = oneQuarterOfPagesize * 3
)

func newBuffer(n int) (b Buffer) {
	buf := &buffer{}
	buf.grow(n)
	b = buf
	return
}

type buffer struct {
	buf []byte
	cap int
	r   int // next position to read
	w   int // next position to write
	h   int // next position to apply

}

func (buf *buffer) Len() (n int) {
	n = buf.w - buf.r
	return
}

func (buf *buffer) Cap() (n int) {
	n = buf.cap
	return
}

func (buf *buffer) Available() (n int) {
	n = buf.cap - buf.h
	return
}

func (buf *buffer) Peek(n int) (p []byte) {
	if n < 1 || buf.Len() == 0 {
		return
	}
	if buf.Len() > n {
		p = buf.buf[buf.r : buf.r+n]
		return
	}
	p = buf.buf[buf.r:buf.w]
	return
}

func (buf *buffer) Next(n int) (p []byte, err error) {
	if n < 1 {
		return
	}
	if buf.Empty() {
		err = io.EOF
		return
	}
	b := buf.Peek(n)
	peeked := len(b)
	if peeked == 0 {
		return
	}
	p = make([]byte, peeked)
	copy(p, b)
	buf.Discard(n)
	return
}

func (buf *buffer) Discard(n int) {
	if n < 1 || buf.Len() == 0 {
		return
	}
	if buf.Len() <= n {
		buf.tryReset()
		return
	}
	nr := buf.r + n
	if nr > buf.w {
		nr = buf.w
	}
	buf.r = nr
	buf.tryReset()
	return
}

func (buf *buffer) Read(p []byte) (n int, err error) {
	pLen := len(p)
	if pLen == 0 {
		return
	}
	if buf.Empty() {
		err = io.EOF
		return
	}
	bufLen := buf.Len()
	if bufLen == 0 {
		return
	}
	if pLen <= bufLen {
		copy(p, buf.buf[buf.r:buf.r+pLen])
		n = pLen
	} else {
		copy(p, buf.buf[buf.r:buf.w])
		n = bufLen
	}
	buf.Discard(n)
	return
}

func (buf *buffer) Write(p []byte) (n int, err error) {
	if !buf.canWrite() {
		panic("bytebuffurpool.Buffer: cannot write, cause prev ApplyAreaForWrite was not finished, please call finish() after the area was wrote")
		return
	}
	n = len(p)
	if buf.Available() < n {
		buf.grow(n)
	}
	buf.buf = append(buf.buf, p...)
	buf.w += n
	buf.h = buf.w
	return
}

func (buf *buffer) WriteString(s string) (n int, err error) {
	if !buf.canWrite() {
		panic("bytebuffurpool.Buffer: cannot write string, cause prev ApplyAreaForWrite was not finished, please call finish() after the area was wrote")
		return
	}
	p := unsafe.Slice(unsafe.StringData(s), len(s))
	n, err = buf.Write(p)
	return
}

func (buf *buffer) WriteByte(c byte) (err error) {
	if !buf.canWrite() {
		panic("bytebuffurpool.Buffer: cannot write byte, cause prev ApplyAreaForWrite was not finished, please call finish() after the area was wrote")
		return
	}
	if buf.Available() < 1 {
		buf.grow(1)
	}
	buf.buf = append(buf.buf, c)
	buf.w++
	buf.h = buf.w
	return
}

func (buf *buffer) WriteRune(r rune) (n int, err error) {
	if !buf.canWrite() {
		panic("bytebuffurpool.Buffer: cannot write rune, cause prev ApplyAreaForWrite was not finished, please call finish() after the area was wrote")
		return
	}
	if uint32(r) < utf8.RuneSelf {
		err = buf.WriteByte(byte(r))
		if err != nil {
			return
		}
		return 1, nil
	}
	p := make([]byte, 0, utf8.UTFMax)
	p = utf8.AppendRune(p, r)
	n, err = buf.Write(p)
	return
}

func (buf *buffer) ApplyAreaForWrite(n int) (area AreaOfBuffer) {
	if !buf.canWrite() {
		panic("bytebuffurpool.Buffer: cannot apply area for write, cause prev ApplyAreaForWrite was not finished, please call finish() after the area was wrote")
		return
	}
	if n < 1 {
		n = oneQuarterOfPagesize
	}
	if buf.Available() < n {
		buf.grow(n)
	}
	buf.buf = append(buf.buf, make([]byte, n)...)
	buf.h += n
	area = &areaOfBuffer{
		p:      buf.buf[buf.w:buf.h],
		finish: buf.FinishAreaWrite,
		cancel: buf.CancelAreaWrite,
	}
	return
}

func (buf *buffer) FinishAreaWrite() {
	if buf.h <= buf.w {
		panic("bytebuffurpool.Buffer: cannot finish area write, cause not applied")
		return
	}
	buf.w = buf.h
}

func (buf *buffer) CancelAreaWrite() {
	if buf.h <= buf.w {
		return
	}
	buf.h = buf.w
}

func (buf *buffer) Reset() {
	buf.tryReset()
	return
}

func (buf *buffer) grow(n int) {
	if n < 1 {
		n = pageszie
	}
	adjustedSize := int(math.Ceil(float64(n)/float64(pageszie)) * float64(pageszie))
	buf.buf = slices.Grow(buf.buf, adjustedSize)
	buf.cap += adjustedSize
	return
}

func (buf *buffer) canWrite() (ok bool) {
	ok = buf.w == buf.h
	return
}

func (buf *buffer) tryReset() {
	if buf.w != buf.h {
		// ptr of h position can not be changed
		// so skip reset
		return
	}
	if buf.w < threeQuarterOfPagesize {
		return
	}
	if halfOfPagesize < buf.w-buf.r {
		return
	}
	if buf.r == buf.w {
		buf.r = 0
		buf.w = 0
		buf.h = 0
		buf.buf = buf.buf[0:0]
		return
	}
	buf.buf = append(buf.buf[0:0], buf.buf[buf.r:buf.w]...)
	buf.w -= buf.r
	buf.h = buf.w
	buf.r = 0
	return
}

func (buf *buffer) WritePending() bool {
	return buf.h > buf.w
}

func (buf *buffer) Empty() bool {
	return buf.w-buf.r == 0 && buf.h == buf.w
}
