//go:build !linux

package bytebuffers

import "runtime"

func (buf *buffer) Close() (err error) {
	runtime.SetFinalizer(buf, nil)
	return
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

	if buf.b != nil {
		n = n - buf.r
		// left shift
		copy(buf.b, buf.b[buf.r:buf.w])
		buf.w -= buf.r
		buf.a = buf.w
		buf.r = 0
		if n < 1 { // has place for n
			return
		}
	}

	// has no more place
	adjustedSize := adjustBufferSize(n)
	buf.b = append(buf.b, make([]byte, adjustedSize)...)
	buf.c += adjustedSize
	return
}
