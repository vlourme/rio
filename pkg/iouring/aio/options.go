package aio

import (
	"strings"
	"time"
)

type Options struct {
	Entries                    uint32
	Flags                      uint32
	SQThreadCPU                uint32
	SQThreadIdle               uint32
	RegisterFixedBufferSize    uint32
	RegisterFixedBufferCount   uint32
	RegisterFixedFiles         uint32
	RegisterReservedFixedFiles uint32
	PrepSQEAffCPU              int
	PrepSQEBatchMinSize        uint32
	PrepSQEBatchTimeWindow     time.Duration
	PrepSQEBatchIdleTime       time.Duration
	WaitCQEMode                string
	WaitCQETimeCurve           Curve
	waitCQEPullIdleTime        time.Duration
	AttachRingFd               int
}

type Option func(*Options)

// WithAttach
// attach ring.
// see https://man.archlinux.org/man/extra/liburing/io_uring_setup.2.en#IORING_SETUP_ATTACH_WQ.
func WithAttach(v *Vortex) Option {
	return func(o *Options) {
		if v == nil {
			return
		}
		fd := v.Fd()
		if fd < 1 {
			return
		}
		o.AttachRingFd = fd
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

// WithSQThreadCPU
// setup iouring's sq thread cpu.
func WithSQThreadCPU(cpuId uint32) Option {
	return func(opts *Options) {
		opts.SQThreadCPU = cpuId
	}
}

const (
	defaultSQThreadIdle = 10000
)

// WithSQThreadIdle
// setup iouring's sq thread idle, the unit is millisecond.
func WithSQThreadIdle(idle time.Duration) Option {
	return func(opts *Options) {
		if idle < time.Millisecond {
			idle = defaultSQThreadIdle * time.Millisecond
		}
		opts.SQThreadIdle = uint32(idle.Milliseconds())
	}
}

// WithPrepSQEAFFCPU
// setup affinity cpu of preparing sqe.
func WithPrepSQEAFFCPU(cpu int) Option {
	return func(opts *Options) {
		opts.PrepSQEAffCPU = cpu
	}
}

// WithPrepSQEBatchMinSize
// setup min batch for preparing sqe.
func WithPrepSQEBatchMinSize(size uint32) Option {
	return func(opts *Options) {
		if size < 1 {
			size = 64
		}
		opts.PrepSQEBatchMinSize = size
	}
}

const (
	defaultPrepSQEBatchTimeWindow = 100 * time.Microsecond
)

// WithPrepSQEBatchTimeWindow
// setup time window of batch for preparing sqe without SQ_POLL.
func WithPrepSQEBatchTimeWindow(window time.Duration) Option {
	return func(opts *Options) {
		if window < 1 {
			window = defaultPrepSQEBatchTimeWindow
		}
		opts.PrepSQEBatchTimeWindow = window
	}
}

const (
	defaultPrepSQEBatchIdleTime = 30 * time.Second
)

// WithPrepSQEBatchIdleTime
// setup idle time for preparing sqe without SQ_POLL.
func WithPrepSQEBatchIdleTime(d time.Duration) Option {
	return func(opts *Options) {
		if d < 1 {
			d = defaultPrepSQEBatchIdleTime
		}
		opts.PrepSQEBatchIdleTime = d
	}
}

const (
	WaitCQEPushMode = "PUSH"
	WaitCQEPullMode = "PULL"
)

// WithWaitCQEMode
// setup mode of wait cqe, default is [WaitCQEPushMode]
func WithWaitCQEMode(mode string) Option {
	return func(opts *Options) {
		mode = strings.ToUpper(strings.TrimSpace(mode))
		if mode == WaitCQEPushMode || mode == WaitCQEPullMode {
			opts.WaitCQEMode = mode
		}
	}
}

// WithWaitCQETimeCurve
// setup time curve for waiting cqe.
func WithWaitCQETimeCurve(curve Curve) Option {
	return func(opts *Options) {
		opts.WaitCQETimeCurve = curve
	}
}

const (
	defaultWaitCQEPullIdleTime = 15 * time.Second
)

// WithWaitCQEPullIdleTime
// setup idle time for pull wait mode.
func WithWaitCQEPullIdleTime(d time.Duration) Option {
	return func(opts *Options) {
		if d < 1 {
			d = defaultWaitCQEPullIdleTime
		}
		opts.waitCQEPullIdleTime = d
	}
}

// WithRegisterFixedBuffer
// setup register fixed buffer of iouring.
func WithRegisterFixedBuffer(size uint32, count uint32) Option {
	return func(opts *Options) {
		if size == 0 || count == 0 {
			return
		}
		opts.RegisterFixedBufferSize = size
		opts.RegisterFixedBufferCount = count
	}
}

// WithRegisterFixedFiles
// setup register fixed fd of iouring.
func WithRegisterFixedFiles(files uint32) Option {
	return func(opts *Options) {
		opts.RegisterFixedFiles = files
	}
}

// WithRegisterReservedFixedFiles
// setup  register reserved fixed fd of iouring.
func WithRegisterReservedFixedFiles(files uint32) Option {
	return func(opts *Options) {
		opts.RegisterReservedFixedFiles = files
	}
}
