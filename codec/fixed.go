package codec

import (
	"context"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"io"
)

func FixedDecode(ctx context.Context, reader transport.Reader, fixed int, options ...async.Option) (future async.Future[[]byte]) {
	decoder := NewFixedEncoder(fixed)
	future = Decode[[]byte](ctx, reader, decoder, options...)
	return
}

func FixedEncode(ctx context.Context, writer transport.Writer, b []byte, fixed int) (future async.Future[int]) {
	encoder := NewLengthFieldEncoder(fixed)
	encoded, encodeErr := encoder.Encode(b)
	if encodeErr != nil {
		future = async.FailedImmediately[int](ctx, encodeErr)
		return
	}
	future = writer.Write(encoded)
	return
}

func NewFixedEncoder(fixed int) *FixedEncoder {
	if fixed < 1 {
		panic("codec.FixedEncoder: fixed must be > 0")
		return nil
	}
	return &FixedEncoder{
		n: fixed,
	}
}

type FixedEncoder struct {
	n int
}

func (encoder *FixedEncoder) Encode(param []byte) (b []byte, err error) {
	pLen := len(param)
	n := encoder.n
	if pLen < encoder.n {
		n = pLen
	}
	b = make([]byte, encoder.n)
	copy(b, param[0:n])
	return
}

func (encoder *FixedEncoder) Decode(inbound transport.Inbound) (ok bool, message []byte, err error) {
	if n := inbound.Received(); n == 0 {
		return
	}

	buf := inbound.Reader()
	if buf == nil {
		// when reading, buf must not be nil
		// only conn closed, then buf will be nil
		// so return io.ErrUnexpectedEOF
		err = io.ErrUnexpectedEOF
		return
	}

	bufLen := buf.Length()
	if bufLen < encoder.n {
		return
	}

	message = make([]byte, encoder.n)
	rn, rErr := inbound.Reader().Read(message)
	if rErr != nil {
		err = rErr
		return
	}
	if rn != encoder.n {
		err = io.ErrShortBuffer
		return
	}
	ok = true
	return
}
