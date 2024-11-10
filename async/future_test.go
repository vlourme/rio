package async_test

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/async"
	"sync"
	"testing"
	"time"
)

func TestTryPromise(t *testing.T) {
	exec := async.New()
	defer exec.CloseGracefully()
	ctx := async.With(context.Background(), exec)
	promise, ok := async.TryPromise[int](ctx)
	if !ok {
		t.Errorf("try promise failed")
		return
	}
	promise.Succeed(1)
	future := promise.Future()
	future.OnComplete(func(ctx context.Context, result int, err error) {
		t.Log("future result:", result, err)
	})
}

func TestMustPromise(t *testing.T) {
	exec := async.New()
	defer exec.CloseGracefully()
	ctx := async.With(context.Background(), exec)
	promise, err := async.MustPromise[int](ctx)
	if err != nil {
		t.Errorf("must promise failed")
		return
	}
	promise.Succeed(1)
	future := promise.Future()
	wg := new(sync.WaitGroup)
	wg.Add(1)
	future.OnComplete(func(ctx context.Context, result int, err error) {
		t.Log("future result:", result, err)
		wg.Done()
	})
	wg.Wait()
}

func TestTryPromise_CompleteErr(t *testing.T) {
	exec := async.New()
	defer exec.Close()
	ctx := async.With(context.Background(), exec)
	promise, ok := async.TryPromise[int](ctx)
	if !ok {
		t.Errorf("try promise failed")
		return
	}
	promise.Fail(errors.New("complete failed"))
	future := promise.Future()
	wg := new(sync.WaitGroup)
	wg.Add(1)
	future.OnComplete(func(ctx context.Context, result int, err error) {
		t.Log("future result:", result, err)
		wg.Done()
	})
	wg.Wait()
}

func TestTryPromise_Cancel(t *testing.T) {
	exec := async.New()
	defer exec.Close()
	ctx := async.With(context.Background(), exec)
	wg := new(sync.WaitGroup)
	wg.Add(1)
	promise1, ok1 := async.TryPromise[int](ctx)
	if !ok1 {
		t.Errorf("try promise1 failed")
		return
	}
	future1 := promise1.Future()
	future1.OnComplete(func(ctx context.Context, result int, err error) {
		t.Log("future1 result:", result, err)
		wg.Add(1)
		promise2, ok2 := async.TryPromise[int](ctx)
		if !ok2 {
			t.Errorf("try promise2 failed")
		}
		promise2.Succeed(2)
		future2 := promise2.Future()
		future2.OnComplete(func(ctx context.Context, result int, err error) {
			t.Log("future2 result:", result, err)
			wg.Done()
		})
		wg.Done()
	})
	promise1.Cancel()
	wg.Wait()
}

func TestTryPromise_Timeout(t *testing.T) {
	exec := async.New()
	defer exec.Close()
	ctx := async.With(context.Background(), exec)
	wg := new(sync.WaitGroup)
	wg.Add(1)
	promise1, ok1 := async.TryPromise[int](ctx)
	if !ok1 {
		t.Errorf("try promise1 failed")
		return
	}
	promise1.SetDeadline(time.Now().Add(3 * time.Second))
	future1 := promise1.Future()
	future1.OnComplete(func(ctx context.Context, result int, err error) {
		t.Log("future1 result:", result, err)
		wg.Add(1)
		promise2, ok2 := async.TryPromise[int](ctx)
		if !ok2 {
			t.Errorf("try promise2 failed")
		}
		promise2.Succeed(2)
		future2 := promise2.Future()
		future2.OnComplete(func(ctx context.Context, result int, err error) {
			t.Log("future2 result:", result, err)
			wg.Done()
		})
		wg.Done()
	})
	time.Sleep(4 * time.Second)
	promise1.Cancel()
	wg.Wait()
}

func TestPromise_Await(t *testing.T) {
	exec := async.New()
	defer exec.Close()
	ctx := async.With(context.Background(), exec)
	promise, ok := async.TryPromise[int](ctx)
	if !ok {
		t.Errorf("try promise failed")
		return
	}
	promise.Succeed(1)
	future := promise.Future()
	v, err := async.Await[int](future)
	if err != nil {
		t.Errorf("await failed: %v", err)
	}
	t.Log(v)
}

func BenchmarkTryPromise(b *testing.B) {
	b.ReportAllocs()
	exec := async.New()
	defer exec.CloseGracefully()
	ctx := async.With(context.Background(), exec)
	wg := new(sync.WaitGroup)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			promise, ok := async.TryPromise[int](ctx)
			if !ok {
				b.Errorf("try promise failed")
				return
			}
			promise.Succeed(1)
			wg.Add(1)
			promise.Future().OnComplete(func(ctx context.Context, result int, err error) {
				wg.Done()
			})
		}
	})
	wg.Wait()
}
