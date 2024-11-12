package codec

import (
	"context"
	"encoding/binary"
	"errors"
	"github.com/brickingsoft/rio/async"
	"github.com/brickingsoft/rio/transport"
	"io"
)

const (
	lengthFieldSize = 8
)

func LengthFieldDecode(ctx context.Context, reader FutureReader) (future async.Future[[]byte]) {
	decoder := LengthFieldDecoder[[]byte]{
		infinite: false,
	}
	future = Decode[[]byte](ctx, reader, &decoder)
	return
}

func LengthFieldStreamDecode(ctx context.Context, reader FutureReader, buf int) (future async.Future[[]byte]) {
	decoder := LengthFieldDecoder[[]byte]{
		infinite: true,
	}
	future = StreamDecode[[]byte](ctx, reader, &decoder, buf)
	return
}

type LengthFieldDecoder[T []byte] struct {
	infinite bool
}

func (decoder *LengthFieldDecoder[T]) Decode(inbound transport.Inbound) (message T, next bool, err error) {
	n := inbound.Received()
	if n == 0 {
		next = decoder.infinite
		return
	}
	if lengthFieldSize > n {
		// not full
		next = true
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
	if bufLen < n {
		err = io.ErrUnexpectedEOF
		return
	}
	if bufLen < lengthFieldSize {
		// not full
		next = true
		return
	}
	lengthField := buf.Peek(lengthFieldSize)
	size := int(binary.BigEndian.Uint64(lengthField))
	if size == 0 {
		next = decoder.infinite
		return
	}
	if bufLen-lengthFieldSize < size {
		// not full
		next = true
		return
	}
	pLen := lengthFieldSize + size
	p := make([]byte, pLen)
	rn, readErr := buf.Read(p)
	if readErr != nil {
		err = io.ErrUnexpectedEOF
		return
	}
	if rn != pLen {
		err = io.ErrShortBuffer
		return
	}
	message = p[lengthFieldSize:]
	next = decoder.infinite
	return
}

func LengthFieldEncode(ctx context.Context, writer FutureWriter, p []byte) (future async.Future[transport.Outbound]) {
	pLen := len(p)
	if pLen == 0 {
		future = async.FailedImmediately[transport.Outbound](ctx, errors.New("codec: empty packet"))
		return
	}
	b := make([]byte, lengthFieldSize+pLen)
	binary.BigEndian.PutUint64(b, uint64(pLen))
	copy(b[lengthFieldSize:], p)
	future = writer.Write(b)
	return
}
