package aio_test

import (
	"fmt"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"sync"
	"testing"
)

type QueueEntry struct {
	N int
}

func (e *QueueEntry) String() string {
	return fmt.Sprintf("%d", e.N)
}

func TestQueue(t *testing.T) {
	queue := aio.NewQueue[QueueEntry]()
	wg := new(sync.WaitGroup)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(queue *aio.Queue[QueueEntry], i int, wg *sync.WaitGroup) {
			queue.Enqueue(&QueueEntry{N: i})
			wg.Done()
		}(queue, i, wg)
	}
	wg.Wait()

	t.Log("length", queue.Length())
	nn := make([]*QueueEntry, 0, 1)
	for {
		n := queue.Dequeue()
		if n == nil {
			break
		}
		nn = append(nn, n)
	}
	t.Log("dequeue", nn)
	t.Log("length", queue.Length(), queue.Dequeue())

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(queue *aio.Queue[QueueEntry], i int, wg *sync.WaitGroup) {
			queue.Enqueue(&QueueEntry{N: i})
			wg.Done()
		}(queue, i, wg)
	}
	wg.Wait()

	nn = make([]*QueueEntry, 5)
	peeked := queue.PeekBatch(nn)
	t.Log("peeked", peeked, nn)
	t.Log("length", queue.Length())
	queue.Advance(peeked)
	t.Log("length", queue.Length())

	nn = make([]*QueueEntry, 5)
	peeked = queue.PeekBatch(nn)
	t.Log("peeked", peeked, nn)
	t.Log("length", queue.Length())
	queue.Advance(peeked)
	t.Log("length", queue.Length())

	nn = make([]*QueueEntry, 0, 1)
	for {
		n := queue.Dequeue()
		if n == nil {
			break
		}
		nn = append(nn, n)
	}
	t.Log("dequeue", nn)
	t.Log("length", queue.Length())
}
