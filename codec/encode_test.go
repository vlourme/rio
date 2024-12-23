package codec_test

import (
	"bytes"
	"context"
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

func (w *FakeWriter) Write(p []byte) (future async.Future[int]) {
	w.p = p
	future = async.SucceedImmediately[int](w.ctx, len(p))
	return
}

func (w *FakeWriter) Equals(b []byte) bool {
	return bytes.Equal(w.p, b)
}
