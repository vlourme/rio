package iouring_test

import (
	"fmt"
	"github.com/brickingsoft/rio/pkg/iouring"
	"log"
	"runtime"
	"testing"
	"time"
)

const (
	ringSize  = 9192
	batchSize = 4096
	benchTime = 10
)

func TestNewRing(t *testing.T) {
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ring, ringErr := iouring.CreateRing(ringSize)
	if ringErr != nil {
		log.Panic(ringErr)
	}
	defer ring.QueueExit()

	cqeBuff := make([]*iouring.CompletionQueueEvent, batchSize)

	benchTimeExpected := (time.Second * benchTime).Nanoseconds()
	startTime := time.Now().UnixNano()
	var count uint64

	for {
		for i := 0; i < batchSize; i++ {
			entry := ring.GetSQE()
			if entry == nil {
				log.Panic()
			}
			entry.PrepareNop()
		}
		submitted, err := ring.SubmitAndWait(batchSize)
		if err != nil {
			log.Panic(err)
		}

		if batchSize != int(submitted) {
			log.Panicf("Submitted %d, expected %d", submitted, batchSize)
		}

		peeked := ring.PeekBatchCQE(cqeBuff)
		if batchSize != int(peeked) {
			log.Panicf("Peeked %d, expected %d", peeked, batchSize)
		}

		count += uint64(peeked)

		ring.CQAdvance(uint32(submitted))

		nowTime := time.Now().UnixNano()
		elapsedTime := nowTime - startTime

		if elapsedTime > benchTimeExpected {
			duration := time.Duration(elapsedTime * int64(time.Nanosecond))
			// nolint: forbidigo
			fmt.Println("Submitted ", count, " entries in ", duration, " seconds")
			// nolint: forbidigo
			fmt.Println(count/uint64(duration.Seconds()), " ops/s")

			return
		}
	}
}
