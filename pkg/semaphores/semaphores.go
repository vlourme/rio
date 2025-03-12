package semaphores

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

func New(timeout time.Duration) (v *Semaphores, err error) {
	if timeout < 1 {
		err = errors.New("invalid timeout")
		return
	}
	v = &Semaphores{
		timeout: timeout,
		timer:   time.NewTimer(timeout),
		ch:      make(chan struct{}, 1),
		status:  atomic.Bool{},
	}
	return
}

type Semaphores struct {
	timeout time.Duration
	timer   *time.Timer
	ch      chan struct{}
	status  atomic.Bool
}

func (s *Semaphores) Signal() {
	if s.status.CompareAndSwap(true, false) {
		s.ch <- struct{}{}
	}
}

func (s *Semaphores) Wait(ctx context.Context) (err error) {
	if s.status.CompareAndSwap(false, true) {
		s.timer.Reset(s.timeout)
		select {
		case <-ctx.Done():
			err = ctx.Err()
			break
		case <-s.timer.C:
			err = context.DeadlineExceeded
			break
		case _, ok := <-s.ch:
			if !ok {
				err = context.Canceled
			}
			break
		}
		s.timer.Stop()
	}
	return
}

func (s *Semaphores) Close() error {
	s.timer.Stop()
	close(s.ch)
	return nil
}
