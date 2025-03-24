package aio

import (
	"strings"
	"time"
)

type Options struct {
	Entries                     uint32
	Flags                       uint32
	SQThreadCPU                 uint32
	SQThreadIdle                uint32
	RegisterFixedBufferSize     uint32
	RegisterFixedBufferCount    uint32
	RegisterFixedFiles          uint32
	RegisterReservedFixedFiles  uint32
	SQEProducerAffinityCPU      int
	SQEProducerBatchSize        uint32
	SQEProducerBatchTimeWindow  time.Duration
	SQEProducerBatchIdleTime    time.Duration
	CQEConsumerType             string
	CQEConsumeTimeCurve         Curve
	CQEPullTypedConsumeIdleTime time.Duration
	HeartbeatTimeout            time.Duration
	AttachRingFd                int
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

// WithSQEProducerAffinityCPU
// setup affinity cpu of preparing sqe.
func WithSQEProducerAffinityCPU(cpu int) Option {
	return func(opts *Options) {
		opts.SQEProducerAffinityCPU = cpu
	}
}

// WithSQEProducerBatchSize
// setup min batch for preparing sqe.
func WithSQEProducerBatchSize(size uint32) Option {
	return func(opts *Options) {
		if size < 1 {
			size = 64
		}
		opts.SQEProducerBatchSize = size
	}
}

const (
	defaultSQEProduceBatchTimeWindow = 100 * time.Microsecond
)

// WithSQEProducerBatchTimeWindow
// setup time window of batch for preparing sqe without SQ_POLL.
func WithSQEProducerBatchTimeWindow(window time.Duration) Option {
	return func(opts *Options) {
		if window < 1 {
			window = defaultSQEProduceBatchTimeWindow
		}
		opts.SQEProducerBatchTimeWindow = window
	}
}

const (
	defaultSQEProduceBatchIdleTime = 30 * time.Second
)

// WithSQEProducerBatchIdleTime
// setup idle time for preparing sqe without SQ_POLL.
func WithSQEProducerBatchIdleTime(d time.Duration) Option {
	return func(opts *Options) {
		if d < 1 {
			d = defaultSQEProduceBatchIdleTime
		}
		opts.SQEProducerBatchIdleTime = d
	}
}

const (
	CQEConsumerPushType = "PUSH"
	CQEConsumerPollType = "PULL"
)

// WithCQEConsumerType
// setup mode of wait cqe, default is [CQEConsumerPushType]
func WithCQEConsumerType(mode string) Option {
	return func(opts *Options) {
		mode = strings.ToUpper(strings.TrimSpace(mode))
		if mode == CQEConsumerPushType || mode == CQEConsumerPollType {
			opts.CQEConsumerType = mode
		}
	}
}

// WithCQEConsumeTimeCurve
// setup time curve for waiting cqe.
func WithCQEConsumeTimeCurve(curve Curve) Option {
	return func(opts *Options) {
		opts.CQEConsumeTimeCurve = curve
	}
}

const (
	defaultCQEPullTypedConsumeIdleTime = 15 * time.Second
)

// WithCQEPullTypedConsumeIdleTime
// setup idle time for pull wait mode.
func WithCQEPullTypedConsumeIdleTime(d time.Duration) Option {
	return func(opts *Options) {
		if d < 1 {
			d = defaultCQEPullTypedConsumeIdleTime
		}
		opts.CQEPullTypedConsumeIdleTime = d
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

const (
	defaultHeartbeatTimeout = 30 * time.Second
)

// WithHeartBeatTimeout
// setup heartbeat timeout.
func WithHeartBeatTimeout(d time.Duration) Option {
	return func(opts *Options) {
		if d < 1 {
			d = defaultHeartbeatTimeout
		}
		opts.HeartbeatTimeout = d
	}
}
