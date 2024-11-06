package async_test

import (
	"context"
	"github.com/brickingsoft/rio/pkg/async"
	"testing"
)

func TestTryInfinitePromise(t *testing.T) {
	exec := async.New()
	defer exec.Close()
	ctx := async.With(context.Background(), exec)
	promise, ok := async.TryInfinitePromise[*Closer](ctx)
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
		promise.Succeed(&Closer{N: i})
	}
	promise.Cancel()
	<-ctx.Done()
}

type Closer struct {
	N int
}

func (c *Closer) Close() error {
	return nil
}
