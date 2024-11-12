package async

import (
	"time"
)

const (
	defaultMaxGoroutines            = 256 * 1024
	defaultMaxGoroutineIdleDuration = 10 * time.Second
)

type Option func(*Options) error

type Options struct {
	MaxGoroutines            int
	MaxGoroutineIdleDuration time.Duration
}

func MaxGoroutines(max int) Option {
	return func(o *Options) error {
		if max < 1 {
			max = defaultMaxGoroutines
		}
		o.MaxGoroutines = max
		return nil
	}
}

func MaxGoroutineIdleDuration(d time.Duration) Option {
	return func(o *Options) error {
		if d < 1 {
			d = defaultMaxGoroutineIdleDuration
		}
		o.MaxGoroutineIdleDuration = d
		return nil
	}
}
