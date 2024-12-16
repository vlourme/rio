package codec

import (
	"context"
	"encoding/binary"
	"errors"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"io"
)

const (
	lengthFieldSize = 8
)

type LengthFieldMessage struct {
	Length int
	Bytes  []byte
}

func LengthFieldDecode(ctx context.Context, reader FutureReader, options ...async.Option) (future async.Future[LengthFieldMessage]) {
	decoder := LengthFieldDecoder{}
	future = Decode[LengthFieldMessage](ctx, reader, &decoder, options...)
	return
}

type LengthFieldDecoder struct {
}

func (decoder *LengthFieldDecoder) Decode(inbound transport.Inbound) (ok bool, message LengthFieldMessage, err error) {
	n := inbound.Received()
	if n == 0 {
		return
	}
	if lengthFieldSize > n {
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
	bufLen := buf.Length()
	if bufLen < n {
		err = io.ErrUnexpectedEOF
		return
	}
	if bufLen < lengthFieldSize {
		// not full
		return
	}
	lengthField := buf.Peek(lengthFieldSize)
	size := int(binary.BigEndian.Uint64(lengthField))
	if size == 0 {
		// decoded but content size is zero
		// so discard length field
		buf.Discard(lengthFieldSize)
		ok = true
		return
	}
	if bufLen-lengthFieldSize < size {
		// not full
		return
	}
	pLen := lengthFieldSize + size
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
	message.Bytes = p[lengthFieldSize:]
	ok = true
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
