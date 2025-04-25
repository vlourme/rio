package aio

import (
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

// WithSendZCEnabled
// setup to use send_zc and sendmsg_zc op insteadof send and sendmsg
func WithSendZCEnabled(enabled bool) Option {
	return func(options *Options) {
		options.SendZCEnabled = enabled
	}
}

// WithMultishotDisabled
// setup to disable multishot
func WithMultishotDisabled(disabled bool) Option {
	return func(options *Options) {
		options.MultishotDisabled = disabled
	}
}

// WithBufferAndRingConfig
// setup buffer and ring config
func WithBufferAndRingConfig(size int, count int, idleTimeout time.Duration) Option {
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
