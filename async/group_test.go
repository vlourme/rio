package async_test

import (
	"context"
	"github.com/brickingsoft/rio/async"
	"sync"
	"testing"
)

func TestGroup(t *testing.T) {
	exec := async.New()
	defer exec.Close()
	ctx := async.With(context.Background(), exec)
	wg := new(sync.WaitGroup)

	promises := make([]async.Promise[int], 0, 1)
	for i := 0; i < 10; i++ {
		promise, ok := async.TryPromise[int](ctx)
		if !ok {
			t.Errorf("try promise failed")
			return
		}
		promises = append(promises, promise)
	}

	group := async.Group[int](promises)
	group.OnComplete(func(ctx context.Context, result int, err error) {
		t.Log("group:", result, err)
		wg.Done()
	})

	for i, promise := range promises {
		wg.Add(1)
		promise.Succeed(i)
	}

	wg.Wait()
}
