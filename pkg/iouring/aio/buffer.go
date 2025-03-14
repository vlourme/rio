package aio

import "io"

type FixedBuffer struct {
	value  []byte
	index  int
	rPos   int
	wPos   int
	ringId int
}

func (buf *FixedBuffer) RingId() int {
	return buf.ringId
}

func (buf *FixedBuffer) Validate() bool {
	return buf.index > -1 && len(buf.value) > 0
}

func (buf *FixedBuffer) Index() int {
	return buf.index
}

func (buf *FixedBuffer) Length() int {
	return buf.wPos - buf.rPos
}

func (buf *FixedBuffer) Reset() {
	buf.rPos = 0
	buf.wPos = 0
}

func (buf *FixedBuffer) rightShiftReadPosition(n int) {
	buf.rPos += n
}

func (buf *FixedBuffer) rightShiftWritePosition(n int) {
	buf.wPos += n
}

func (buf *FixedBuffer) Write(b []byte) (n int, err error) {
	n = copy(buf.value[buf.wPos:], b)
	buf.wPos += n
	return
}

func (buf *FixedBuffer) Read(b []byte) (int, error) {
	if buf.wPos-buf.rPos == 0 {
		return 0, io.EOF
	}
	if len(b) == 0 {
		return 0, nil
	}
	n := copy(b, buf.value[buf.rPos:buf.wPos])
	buf.rPos += n
	return n, nil
}
