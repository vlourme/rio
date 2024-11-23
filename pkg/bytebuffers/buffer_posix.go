//go:build !linux

package bytebuffers

import (
	"bytes"
	"io"
	"math"
)

func NewBuffer() Buffer {
	return NewBufferWitSize(1)
}

func NewBufferWitSize(size int) Buffer {
	if size <= 0 {
		size = 1
	}
	b := &buffer{
		b: nil,
		r: 0,
		w: 0,
		a: 0,
	}
	_ = b.grow(size)
	return b
}

type buffer struct {
	b []byte
	r int
	w int
	a int
}

func (buf *buffer) Len() int { return buf.w - buf.r }

func (buf *buffer) Cap() int { return cap(buf.b) }

func (buf *buffer) Peek(n int) (p []byte) {
	bLen := buf.Len()
	if n < 1 || bLen == 0 {
		return
	}
	if bLen > n {
		p = buf.b[buf.r : buf.r+n]
		return
	}
	p = buf.b[buf.r:buf.w]
	return
}

func (buf *buffer) Next(n int) (p []byte, err error) {
	if n < 1 {
		return
	}
	bLen := buf.Len()
	if bLen == 0 && buf.a-buf.w == 0 {
		err = io.EOF
		return
	}
	if n > bLen {
		n = bLen
	}
	p = make([]byte, n)
	data := buf.b[buf.r : buf.r+n]
	copy(p, data)
	buf.r += n

	buf.tryReset()
	return
}

func (buf *buffer) Read(p []byte) (n int, err error) {
	bLen := buf.Len()
	if bLen == 0 && buf.a-buf.w == 0 {
		buf.Reset()
		err = io.EOF
		return
	}
	if len(p) == 0 {
		return
	}
	n = copy(p, buf.b[buf.r:buf.w])
	buf.r += n

	buf.tryReset()
	return
}

func (buf *buffer) Discard(n int) (err error) {
	if n < 1 {
		return
	}
	bLen := buf.Len()
	if bLen == 0 {
		return
	}
	if bLen <= n && buf.a-buf.w == 0 {
		buf.Reset()
		return
	}
	buf.r += n

	buf.tryReset()
	return
}

func (buf *buffer) Write(p []byte) (n int, err error) {
	if buf.WritePending() {
		err = ErrWriteBeforeAllocatedWrote
		return
	}
	pLen := len(p)
	if pLen == 0 {
		return
	}

	if m := buf.w + pLen - buf.Cap(); m > 0 {
		if err = buf.grow(m); err != nil {
			return
		}
	}

	n = copy(buf.b[buf.w:], p)
	buf.w += n
	buf.a = buf.w
	return
}

func (buf *buffer) WritePending() bool {
	return buf.a != buf.w
}

func (buf *buffer) Allocate(size int) (p []byte, err error) {
	if buf.WritePending() {
		err = ErrWriteBeforeAllocatedWrote
		return
	}
	if size < 1 {
		err = ErrAllocateZero
		return
	}
	if m := buf.w + size - buf.Cap(); m > 0 {
		if err = buf.grow(m); err != nil {
			return
		}
	}
	buf.a += size
	p = buf.b[buf.w : buf.w+size]
	return
}

func (buf *buffer) AllocatedWrote(n int) (err error) {
	if buf.a == buf.w {
		return
	}
	if n == 0 {
		buf.a = buf.w
	} else {
		buf.w += n
		buf.a = buf.w
	}
	return
}

func (buf *buffer) Reset() {
	buf.r = 0
	buf.w = 0
	buf.a = 0
}

func (buf *buffer) ReadBytes(delim byte) (line []byte, err error) {
	bLen := buf.Len()
	if bLen == 0 {
		if buf.a == buf.w {
			err = io.EOF
		}
		return
	}
	i := bytes.IndexByte(buf.b[buf.r:buf.w], delim)
	end := buf.r + i + 1
	if i < 0 {
		end = buf.w
		err = io.EOF
	}
	line = buf.b[buf.r:end]
	buf.r = end
	return
}

func (buf *buffer) tryReset() {
	if buf.r == buf.w && buf.a == buf.w {
		buf.Reset()
	}
}

func (buf *buffer) grow(n int) (err error) {
	if n < 1 {
		return
	}
	defer func() {
		if recover() != nil {
			err = ErrTooLarge
		}
	}()

	adjustedSize := int(math.Ceil(float64(n)/float64(pageszie)) * float64(pageszie))
	buf.b = append(buf.b, make([]byte, adjustedSize)...)
	return
}
