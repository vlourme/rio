package aio

import (
	"time"
)

type Options struct {
	Entries             uint32
	Flags               uint32
	SQThreadIdle        uint32
	SendZCEnabled       bool
	MultishotDisabled   bool
	BufferAndRingConfig BufferAndRingConfig
	WaitCQEIdleTimeout  time.Duration
	WaitCQETimeCurve    Curve
}

type Option func(*Options)

// WithEntries
// setup iouring's entries.
func WithEntries(entries uint32) Option {
	return func(opts *Options) {
		opts.Entries = entries
	}
}

// WithFlags
// setup iouring's flags.
// see https://man.archlinux.org/listing/extra/liburing/
func WithFlags(flags uint32) Option {
	return func(opts *Options) {
		opts.Flags |= flags
	}
}

// WithSQThreadIdle
// setup iouring's sq thread idle, the unit is millisecond.
func WithSQThreadIdle(idle time.Duration) Option {
	return func(opts *Options) {
		if idle < time.Millisecond {
			idle = 10000 * time.Millisecond
		}
		opts.SQThreadIdle = uint32(idle.Milliseconds())
	}
}

// WithSendZCEnabled
// setup to use send_zc and sendmsg_zc op insteadof send and sendmsg
func WithSendZCEnabled(enabled bool) Option {
	return func(options *Options) {
		options.SendZCEnabled = enabled
	}
}

// WithMultiShotDisabled
// setup to disable multishot
func WithMultiShotDisabled(disabled bool) Option {
	return func(options *Options) {
		options.MultishotDisabled = disabled
	}
}

// WithRingBufferConfig
// setup buffer and ring config
func WithRingBufferConfig(size int, count int, idleTimeout time.Duration) Option {
	return func(opts *Options) {
		if size < 0 {
			size = 0
		}
		if count < 0 {
			count = 0
		}
		if idleTimeout < 0 {
			idleTimeout = 0
		}
		opts.BufferAndRingConfig = BufferAndRingConfig{
			Size:        size,
			Count:       count,
			IdleTimeout: idleTimeout,
		}
	}
}

// WithWaitCQEIdleTimeout
// setup wait cqe idle timeout
func WithWaitCQEIdleTimeout(timeout time.Duration) Option {
	return func(opts *Options) {
		opts.WaitCQEIdleTimeout = timeout
	}
}

// WithWaitCQETimeCurve
// setup wait cqe time curve
func WithWaitCQETimeCurve(curve Curve) Option {
	return func(opts *Options) {
		opts.WaitCQETimeCurve = curve
	}
}
