package codec_test

import (
	"context"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"io"
)

func newFakeReader(ctx context.Context, p []byte) *FakeReader {
	return &FakeReader{
		ctx: ctx,
		p:   p,
	}
}

type FakeReader struct {
	ctx context.Context
	p   []byte
}

func (r *FakeReader) Read() (future async.Future[transport.Inbound]) {
	if len(r.p) == 0 {
		future = async.FailedImmediately[transport.Inbound](r.ctx, io.EOF)
		return
	}
	buf := transport.NewInboundBuffer()
	n, _ := buf.Write(r.p)
	r.p = r.p[n:]
	future = async.SucceedImmediately[transport.Inbound](r.ctx, transport.NewInbound(buf, n))
	return
}
