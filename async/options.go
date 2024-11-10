package async

import (
	"time"
)

const (
	defaultMaxExecutors            = 256 * 1024
	defaultMaxExecutorIdleDuration = 10 * time.Second
)

type Option func(*Options) error

type Options struct {
	MaxExecutors            int
	MaxExecutorIdleDuration time.Duration
}

func MaxExecutors(max int) Option {
	return func(o *Options) error {
		if max < 1 {
			max = defaultMaxExecutors
		}
		o.MaxExecutors = max
		return nil
	}
}

func MaxIdleExecutorDuration(d time.Duration) Option {
	return func(o *Options) error {
		if d < 1 {
			d = defaultMaxExecutorIdleDuration
		}
		o.MaxExecutorIdleDuration = d
		return nil
	}
}
