package async_test

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/async"
	"sync"
	"testing"
	"time"
)

func TestTryPromise(t *testing.T) {
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
	wg := new(sync.WaitGroup)
	wg.Add(1)
	future.OnComplete(func(ctx context.Context, result int, err error) {
		t.Log("future result:", result, err)
		wg.Done()
	})
	wg.Wait()
}

func TestMustPromise(t *testing.T) {
	exec := async.New()
	defer exec.Close()
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
	wg := new(sync.WaitGroup)
	ctx := context.Background()
	ctx = async.With(ctx, exec)
	promise, ok := async.TryPromise[int](ctx)
	if !ok {
		t.Errorf("try promise failed")
		return
	}
	promise.Cancel()
	wg.Add(1)

	promise.Future().OnComplete(func(ctx context.Context, result int, err error) {
		t.Log("future result:", result, err, "canceled:", async.IsCanceled(err))
		wg.Done()
	})
	wg.Wait()
}

func TestTryPromise_Timeout(t *testing.T) {
	exec := async.New()
	defer exec.Close()
	wg := new(sync.WaitGroup)
	ctx := context.Background()
	ctx = async.With(ctx, exec)
	promise, ok := async.TryPromise[int](ctx)
	if !ok {
		t.Errorf("try promise failed")
		return
	}
	promise.SetDeadline(time.Now().Add(1 * time.Second))
	time.Sleep(2 * time.Second)
	promise.Succeed(1)
	wg.Add(1)

	promise.Future().OnComplete(func(ctx context.Context, result int, err error) {
		t.Log("future result:", result, err, "timeout:", async.IsTimeout(err))
		wg.Done()
	})

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
	v, err := future.Await()
	if err != nil {
		t.Errorf("await failed: %v", err)
	}
	t.Log(v)
}

func BenchmarkTryPromise(b *testing.B) {
	b.ReportAllocs()
	exec := async.New()
	defer exec.Close()
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
