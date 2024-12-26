package codec

import (
	"context"
	"encoding/binary"
	"errors"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"io"
)

type LengthFieldMessage struct {
	Length int
	Bytes  []byte
}

func LengthFieldDecode(ctx context.Context, reader transport.Reader, lengthFieldSize int, options ...async.Option) (future async.Future[LengthFieldMessage]) {
	decoder := NewLengthFieldEncoder(lengthFieldSize)
	future = Decode[LengthFieldMessage](ctx, reader, decoder, options...)
	return
}

func LengthFieldEncode(ctx context.Context, writer transport.Writer, b []byte, lengthFieldSize int) (future async.Future[int]) {
	encoder := NewLengthFieldEncoder(lengthFieldSize)
	encoded, encodeErr := encoder.Encode(b)
	if encodeErr != nil {
		future = async.FailedImmediately[int](ctx, encodeErr)
		return
	}
	future = writer.Write(encoded)
	return
}

func NewLengthFieldEncoder(lengthFieldSize int) *LengthFieldEncoder {
	if lengthFieldSize <= 0 {
		panic("codec.NewLengthFieldEncoder: length field size must be > 0")
		return nil
	}
	return &LengthFieldEncoder{
		lengthFieldSize: lengthFieldSize,
	}
}

type LengthFieldEncoder struct {
	lengthFieldSize int
}

func (encoder *LengthFieldEncoder) Decode(inbound transport.Inbound) (ok bool, message LengthFieldMessage, err error) {
	n := inbound.Received()
	if n == 0 {
		return
	}
	if encoder.lengthFieldSize > n {
		// not full
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

	if bufLen := buf.Length(); bufLen < n {
		n = bufLen
	}
	if n < encoder.lengthFieldSize {
		// not full
		return
	}
	lengthField := buf.Peek(encoder.lengthFieldSize)
	size := int(binary.BigEndian.Uint64(lengthField))
	if size == 0 {
		// decoded but content size is zero
		// so discard length field
		buf.Discard(encoder.lengthFieldSize)
		ok = true
		return
	}
	if n-encoder.lengthFieldSize < size {
		// not full
		return
	}
	pLen := encoder.lengthFieldSize + size
	p := make([]byte, pLen)
	rn, readErr := buf.Read(p)
	if readErr != nil {
		err = readErr
		return
	}
	if rn != pLen {
		err = io.ErrShortBuffer
		return
	}
	message.Length = size
	message.Bytes = p[encoder.lengthFieldSize:]
	ok = true
	return
}

func (encoder *LengthFieldEncoder) Encode(param []byte) (b []byte, err error) {
	bLen := len(b)
	if bLen == 0 {
		err = errors.New("codec.LengthFieldEncoder: empty packet")
		return
	}
	b = make([]byte, encoder.lengthFieldSize+bLen)
	binary.BigEndian.PutUint64(b, uint64(bLen))
	copy(b[encoder.lengthFieldSize:], param)
	return
}
