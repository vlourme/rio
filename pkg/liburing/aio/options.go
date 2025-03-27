package aio

import (
	"time"
)

type Options struct {
	Entries                                     uint32
	Flags                                       uint32
	SQThreadCPU                                 uint32
	SQThreadIdle                                uint32
	DisableDirectAllocFeatKernelFlavorBlackList []string
	RegisterFixedFiles                          uint32
	SQEProducerAffinityCPU                      int
	SQEProducerLockOSThread                     bool
	SQEProducerBatchSize                        uint32
	SQEProducerBatchTimeWindow                  time.Duration
	SQEProducerBatchIdleTime                    time.Duration
	CQEConsumerType                             string
	CQEPullTypedConsumeTimeCurve                Curve
	CQEPullTypedConsumeIdleTime                 time.Duration
	HeartbeatTimeout                            time.Duration
	AttachRingFd                                int
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

// WithDisableDirectAllocFeatKernelFlavorBlackList
// setup disable iouring direct alloc feat kernel flavor black list.
func WithDisableDirectAllocFeatKernelFlavorBlackList(list []string) Option {
	return func(opts *Options) {
		opts.DisableDirectAllocFeatKernelFlavorBlackList = list
	}
}

// WithSQEProducerLockOSThread
// setup lock os thread of producing sqe.
func WithSQEProducerLockOSThread(lock bool) Option {
	return func(opts *Options) {
		opts.SQEProducerLockOSThread = lock
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
	CQEConsumerPullType = "PULL"
)

// WithCQEPushTypedConsumer use push typed cqe consumer
func WithCQEPushTypedConsumer() Option {
	return func(opts *Options) {
		opts.CQEConsumerType = CQEConsumerPushType
	}
}

// WithCQEPullTypedConsumer use pull typed cqe consumer
func WithCQEPullTypedConsumer(curve Curve, idleTime time.Duration) Option {
	return func(opts *Options) {
		opts.CQEConsumerType = CQEConsumerPullType
		opts.CQEPullTypedConsumeTimeCurve = curve
		if idleTime < 1 {
			idleTime = defaultCQEPullTypedConsumeIdleTime
		}
		opts.CQEPullTypedConsumeIdleTime = idleTime
	}
}

// WithRegisterFixedFiles
// setup register fixed fd of iouring.
func WithRegisterFixedFiles(files uint32) Option {
	return func(opts *Options) {
		opts.RegisterFixedFiles = files
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
