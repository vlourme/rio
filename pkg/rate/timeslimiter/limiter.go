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
			if n := bucket.tokens.Add(1); n <= bucket.upperbound {
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

func (bucket *Bucket) Tokens() int64 {
	return bucket.tokens.Load()
}

func (bucket *Bucket) ok() bool {
	return bucket.upperbound > 0
}

type ctxKey struct{}

var (
	key = ctxKey{}
)

func With(ctx context.Context, bucket *Bucket) context.Context {
	return context.WithValue(ctx, key, bucket)
}

func TryFrom(ctx context.Context) (bucket *Bucket, has bool) {
	value := ctx.Value(key)
	if value == nil {
		return
	}
	bucket, has = value.(*Bucket)
	return
}

func From(ctx context.Context) *Bucket {
	bucket, has := TryFrom(ctx)
	if !has {
		panic("timeslimiter: get bucket from context failed  cause the value is not a *Bucket")
		return nil
	}
	return bucket
}

func Wait(ctx context.Context) (err error) {
	bucket := From(ctx)
	err = bucket.Wait(ctx)
	return
}

func Revert(ctx context.Context) {
	bucket := From(ctx)
	bucket.Revert()
}

func TryRevert(ctx context.Context) {
	bucket, has := TryFrom(ctx)
	if has {
		bucket.Revert()
	}
}

func Tokens(ctx context.Context) int64 {
	bucket, has := TryFrom(ctx)
	if has {
		return bucket.Tokens()
	}
	return 0
}
