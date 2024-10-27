package concurrent

import (
	"fmt"
	"time"
)

const (
	defaultMaxWorkers            = 256 * 1024
	defaultMaxIdleWorkerDuration = 2 * time.Second
)

type Option func(*Options) error

type Options struct {
	MaxExecutors           int
	MaxIdleExecuteDuration time.Duration
}

func MaxExecutors(max int) Option {
	return func(o *Options) error {
		if max < 1 {
			return fmt.Errorf("max executors must great than 0")
		}
		o.MaxExecutors = max
		return nil
	}
}

func MaxIdleExecuteDuration(d time.Duration) Option {
	return func(o *Options) error {
		if d < 1 {
			return fmt.Errorf("max idle duration must great than 0")
		}
		o.MaxIdleExecuteDuration = d
		return nil
	}
}
