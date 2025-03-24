package queue_test

import (
	"context"
	"fmt"
	"github.com/brickingsoft/rio/pkg/iouring/aio/queue"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type ValueWithVersion struct {
	value   int
	version uint64
}

func TestVP(t *testing.T) {
	var sharedValue atomic.Value
	sharedValue.Store(ValueWithVersion{value: 10, version: 0})

	// 读取当前值和版本号
	current := sharedValue.Load().(ValueWithVersion)
	fmt.Printf("Current Value: %d, Version: %d\n", current.value, current.version)

	// 尝试CAS操作
	newValue := ValueWithVersion{value: 20, version: current.version + 1}
	success := sharedValue.CompareAndSwap(current, newValue)
	if success {
		fmt.Println("CAS succeeded!")
	} else {
		fmt.Println("CAS failed!")
	}

	// 再次读取值和版本号
	updated := sharedValue.Load().(ValueWithVersion)
	fmt.Printf("Updated Value: %d, Version: %d\n", updated.value, updated.version)
}

type QueueEntry struct {
	N int
}

func (e *QueueEntry) String() string {
	return fmt.Sprintf("%d", e.N)
}

func TestQueue(t *testing.T) {
	q := queue.New[QueueEntry]()
	wg := new(sync.WaitGroup)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(queue *queue.Queue[QueueEntry], i int, wg *sync.WaitGroup) {
			queue.Enqueue(&QueueEntry{N: i})
			wg.Done()
		}(q, i, wg)
	}
	wg.Wait()

	t.Log("length", q.Length())
	nn := make([]*QueueEntry, 0, 1)
	for {
		n := q.Dequeue()
		if n == nil {
			break
		}
		nn = append(nn, n)
	}
	t.Log("dequeue", nn)
	t.Log("length", q.Length(), q.Dequeue())

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(queue *queue.Queue[QueueEntry], i int, wg *sync.WaitGroup) {
			queue.Enqueue(&QueueEntry{N: i})
			wg.Done()
		}(q, i, wg)
	}
	wg.Wait()

	nn = make([]*QueueEntry, 5)
	peeked := q.PeekBatch(nn)
	t.Log("peeked", peeked, nn)
	t.Log("length", q.Length())
	q.Advance(peeked)
	t.Log("length", q.Length())

	nn = make([]*QueueEntry, 5)
	peeked = q.PeekBatch(nn)
	t.Log("peeked", peeked, nn)
	t.Log("length", q.Length())
	q.Advance(peeked)
	t.Log("length", q.Length())

	nn = make([]*QueueEntry, 0, 1)
	for {
		n := q.Dequeue()
		if n == nil {
			break
		}
		nn = append(nn, n)
	}
	t.Log("dequeue", nn)
	t.Log("length", q.Length())
}

func BenchmarkQueue_Enqueue(b *testing.B) {
	/* b
	goos: windows
	goarch: amd64
	pkg: github.com/brickingsoft/rio/pkg/iouring/aio/queue
	cpu: 13th Gen Intel(R) Core(TM) i5-13600K
	BenchmarkQueue_Enqueue
	BenchmarkQueue_Enqueue-20    	10807256	       114.8 ns/op	       0 B/op	       0 allocs/op
	*/
	b.ReportAllocs()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	q := queue.New[QueueEntry]()
	e := &QueueEntry{N: 0}
	for i := 0; i < b.N; i++ {
		e.N = i
		q.Enqueue(e)
		select {
		case <-timer.C:
			break
		default:
			_ = q.Dequeue()
			if ctx.Err() != nil {
				return
			}
			break
		}
		timer.Reset(time.Second)
	}
}

func BenchmarkChan(b *testing.B) {
	/*
		goos: windows
		goarch: amd64
		pkg: github.com/brickingsoft/rio/pkg/iouring/aio/queue
		cpu: 13th Gen Intel(R) Core(TM) i5-13600K
		BenchmarkChan
		BenchmarkChan-20    	11153587	       106.9 ns/op	       0 B/op	       0 allocs/op
	*/
	b.ReportAllocs()
	ch := make(chan int, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for i := 0; i < b.N; i++ {
		ch <- i
		select {
		case <-ch:
			if ctx.Err() != nil {
				return
			}
			break
		case <-timer.C:
			break
		}
		timer.Reset(time.Second)
	}
}

func BenchmarkQueue_Enqueue_P(b *testing.B) {
	/*
		goos: windows
		goarch: amd64
		pkg: github.com/brickingsoft/rio/pkg/iouring/aio/queue
		cpu: 13th Gen Intel(R) Core(TM) i5-13600K
		BenchmarkQueue_Enqueue_P
		BenchmarkQueue_Enqueue_P-20    	 3208533	       383.7 ns/op	       0 B/op	       0 allocs/op
	*/
	b.ReportAllocs()
	q := queue.New[QueueEntry]()
	e := &QueueEntry{N: 0}
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func(q *queue.Queue[QueueEntry], wg *sync.WaitGroup) {
		defer wg.Done()
		time.Sleep(time.Millisecond)
		p := make([]*QueueEntry, 1024)
		for {
			n := q.PeekBatch(p)
			q.Advance(n)
			if n == 0 {
				return
			}

		}
	}(q, wg)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			q.Enqueue(e)
			_ = q.Dequeue()
		}
	})
	wg.Wait()
}

func BenchmarkChan_P(b *testing.B) {
	/*
		goos: windows
		goarch: amd64
		pkg: github.com/brickingsoft/rio/pkg/iouring/aio/queue
		cpu: 13th Gen Intel(R) Core(TM) i5-13600K
		BenchmarkChan_P
		BenchmarkChan_P-20    	 3890246	       313.0 ns/op	       0 B/op	       0 allocs/op
	*/
	b.ReportAllocs()
	wg := new(sync.WaitGroup)
	ch := make(chan int, 1024)
	wg.Add(1)
	go func(ch chan int, wg *sync.WaitGroup) {
		time.Sleep(time.Millisecond)
		defer wg.Done()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		timer := time.NewTicker(1 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-timer.C:
				break
			case <-ctx.Done():
				return
			case _, ok := <-ch:
				if !ok {
					return
				}
			}
		}
	}(ch, wg)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ch <- 1
		}
	})
	close(ch)
	wg.Wait()
}
