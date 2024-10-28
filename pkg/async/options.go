package async

import (
	"time"
)

const (
	defaultMaxExecutors           = 256 * 1024
	defaultMaxExecuteIdleDuration = 2 * time.Second
)

type Option func(*Options) error

type Options struct {
	MaxExecutors           int
	MaxExecuteIdleDuration time.Duration
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

func MaxIdleExecuteDuration(d time.Duration) Option {
	return func(o *Options) error {
		if d < 1 {
			d = defaultMaxExecuteIdleDuration
		}
		o.MaxExecuteIdleDuration = d
		return nil
	}
}
