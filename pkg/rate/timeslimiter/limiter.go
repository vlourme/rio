package timeslimiter

import (
	"context"
	"runtime"
	"sync/atomic"
	"time"
)

func New(upperbound int64) *Bucket {
	if upperbound < 1 {
		upperbound = 0
	}
	return &Bucket{
		upperbound: upperbound,
		tokens:     atomic.Int64{},
	}
}

const (
	ns500    = 500 * time.Nanosecond
	maxTimes = 10
)

type Bucket struct {
	upperbound int64
	tokens     atomic.Int64
}

func (bucket *Bucket) Wait(ctx context.Context) (err error) {
	if !bucket.ok() {
		return
	}
	times := 0
	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		default:
			n := bucket.tokens.Add(1)
			if n <= bucket.upperbound {
				return
			}
			bucket.tokens.Add(-1)
			times++
			if times > maxTimes {
				times = 0
				runtime.Gosched()
			} else {
				time.Sleep(ns500)
			}
		}
	}
}

func (bucket *Bucket) Revert() {
	if !bucket.ok() {
		return
	}
	bucket.tokens.Add(-1)
}

func (bucket *Bucket) Used() int64 {
	return bucket.tokens.Load()
}

func (bucket *Bucket) ok() bool {
	return bucket.upperbound > 0
}
