package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"time"
)

type Options struct {
	EventLoopCount      uint32
	Entries             uint32
	Flags               uint32
	SQThreadIdle        uint32
	SQThreadCPU         uint32
	SendZCEnabled       bool
	MultishotDisabled   bool
	NAPIBusyPollTimeout time.Duration
	BufferAndRingConfig BufferAndRingConfig
	WaitCQETimeoutCurve Curve
}

type Option func(*Options)

// WithEventLoopCount
// setup event loop count
func WithEventLoopCount(count uint32) Option {
	return func(o *Options) {
		o.EventLoopCount = count
	}
}

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

// WithSQPoll
// setup IORING_SETUP_SQPOLL
func WithSQPoll(idleTimeout time.Duration, affCPU int) Option {
	return func(opts *Options) {
		if opts.Flags&liburing.IORING_SETUP_SQPOLL == 0 {
			opts.Flags |= liburing.IORING_SETUP_SQPOLL
		}
		if affCPU > -1 {
			if opts.Flags&liburing.IORING_SETUP_SQ_AFF == 0 {
				opts.Flags |= liburing.IORING_SETUP_SQ_AFF
			}
			opts.SQThreadCPU = uint32(affCPU)
		}
		if idleTimeout < 1*time.Millisecond {
			idleTimeout = 2 * time.Second
		}
		opts.SQThreadIdle = uint32(idleTimeout.Milliseconds())
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
		opts.BufferAndRingConfig.Size = size
		if count < 0 {
			count = 0
		}
		opts.BufferAndRingConfig.Count = count
		if idleTimeout < 0 {
			idleTimeout = 0
		}
		opts.BufferAndRingConfig.IdleTimeout = idleTimeout
	}
}

// WithWaitCQETimeoutCurve
// setup wait cqe time curve
func WithWaitCQETimeoutCurve(curve Curve) Option {
	return func(opts *Options) {
		opts.WaitCQETimeoutCurve = curve
	}
}

// WithNAPIBusyPollTimeout
// setup napi busy poll timeout to register napi, effective in kernel versions higher than 6.8
func WithNAPIBusyPollTimeout(timeout time.Duration) Option {
	return func(opts *Options) {
		opts.NAPIBusyPollTimeout = timeout
	}
}
