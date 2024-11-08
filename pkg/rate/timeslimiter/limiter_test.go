package timeslimiter_test

import (
	"context"
	"github.com/brickingsoft/rio/pkg/rate/timeslimiter"
	"testing"
	"time"
)

func TestBucket_Wait(t *testing.T) {
	bucket := timeslimiter.New(2)
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := bucket.Wait(ctx)
		if err != nil {
			t.Log(err)
		} else {
			t.Log("ok", bucket.Tokens())
		}
		bucket.Revert()
		cancel()
	}
}
