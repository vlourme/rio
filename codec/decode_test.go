package codec_test

import (
	"context"
	"github.com/brickingsoft/rio/pkg/async"
	"github.com/brickingsoft/rio/transport"
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
	buf := transport.NewInboundBuffer()
	n, _ := buf.Write(r.p)
	future = async.SucceedImmediately[transport.Inbound](r.ctx, transport.NewInbound(buf, n))
	return
}
