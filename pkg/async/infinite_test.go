package async_test

import (
	"context"
	"github.com/brickingsoft/rio/pkg/async"
	"sync"
	"testing"
)

func TestTryInfinitePromise(t *testing.T) {
	exec := async.New()
	defer exec.Close()
	ctx := async.With(context.Background(), exec)
	promise, ok := async.TryInfinitePromise[int](ctx)
	if !ok {
		t.Errorf("try promise failed")
		return
	}
	future := promise.Future()
	wg := new(sync.WaitGroup)
	wg.Add(1)
	future.OnComplete(func(ctx context.Context, result int, err error) {
		t.Log("future result:", result, err)
		wg.Done()
	})
	for i := 0; i < 10; i++ {
		promise.Succeed(i)
		wg.Add(1)
	}
	promise.Close()
	wg.Done()
	wg.Wait()
}
