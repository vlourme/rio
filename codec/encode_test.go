package codec_test

import (
	"bytes"
	"context"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
)

func newFakeWriter(ctx context.Context) *FakeWriter {
	return &FakeWriter{
		ctx: ctx,
	}
}

type FakeWriter struct {
	ctx context.Context
	p   []byte
}

func (w *FakeWriter) Write(p []byte) (future async.Future[transport.Outbound]) {
	w.p = p
	future = async.SucceedImmediately[transport.Outbound](w.ctx, transport.NewOutBound(len(p), nil))
	return
}

func (w *FakeWriter) Equals(b []byte) bool {
	return bytes.Equal(w.p, b)
}
