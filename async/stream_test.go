package async_test

import (
	"context"
	"github.com/brickingsoft/rio/async"
	"testing"
)

func TestTryStreamPromise(t *testing.T) {
	exec := async.New()
	defer exec.CloseGracefully()
	ctx := async.With(context.Background(), exec)
	promise, ok := async.TryStreamPromise[*Closer](ctx, 8)
	if !ok {
		t.Errorf("try promise failed")
		return
	}
	future := promise.Future()
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	future.OnComplete(func(ctx context.Context, result *Closer, err error) {
		t.Log("future result:", result, err)
		if err != nil {
			cancel()
			return
		}
		return
	})
	for i := 0; i < 10; i++ {
		promise.Succeed(&Closer{N: i, t: t})
	}
	promise.Cancel()
	<-ctx.Done()
}

type Closer struct {
	N int
	t *testing.T
}

func (c *Closer) Close() error {
	c.t.Log("close ", c.N)
	return nil
}
