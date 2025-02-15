package ring_test

import (
	"github.com/brickingsoft/rio/pkg/ring"
	"testing"
)

func TestOperationQueue_Advance(t *testing.T) {
	queue := ring.NewOperationQueue(5)

	for i := 0; i < 5; i++ {
		op := ring.Operation{}
		op.SetFd(i + 1)
		queue.Enqueue(&op)
	}

	t.Log(queue.Len(), queue.Cap())

	ops := make([]*ring.Operation, 10)
	peeked := queue.PeekBatch(ops)
	t.Log("peeked:", peeked, queue.Len())
	for i := int64(0); i < peeked; i++ {
		t.Log("peeked:", ops[i].Fd())
	}
	queue.Advance(peeked)
	t.Log("Advance:", peeked, queue.Len())

	for i := 0; i < 5; i++ {
		op := ring.Operation{}
		op.SetFd(i + 1)
		queue.Enqueue(&op)
	}
	t.Log(queue.Len(), queue.Cap())

	peeked = queue.PeekBatch(ops)
	t.Log("peeked:", peeked, queue.Len())
	for i := int64(0); i < peeked; i++ {
		t.Log("peeked:", ops[i].Fd())
	}
	queue.Advance(peeked)
	t.Log("Advance:", peeked, queue.Len())
}
