//go:build linux

package aio

import (
	"sync"
	"time"
)

const (
	defaultHeartbeatTimeout = 30 * time.Second
)

func newHeartbeat(timeout time.Duration, producer *operationProducer) *heartbeat {
	if timeout < 1 {
		timeout = defaultHeartbeatTimeout
	}
	hb := &heartbeat{
		producer: producer,
		ticker:   time.NewTicker(timeout),
		wg:       new(sync.WaitGroup),
		done:     make(chan struct{}),
	}
	go hb.start()
	return hb
}

type heartbeat struct {
	producer *operationProducer
	ticker   *time.Ticker
	wg       *sync.WaitGroup
	done     chan struct{}
}

func (hb *heartbeat) start() {
	hb.wg.Add(1)
	defer hb.wg.Done()

	op := &Operation{}
	_ = op.PrepareNop()
	for {
		select {
		case <-hb.done:
			hb.ticker.Stop()
			return
		case <-hb.ticker.C:
			hb.producer.Produce(op)
			break
		}
	}
}

func (hb *heartbeat) Close() error {
	close(hb.done)
	hb.wg.Wait()
	return nil
}
