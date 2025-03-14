package aio

import (
	"strings"
	"time"
)

type Options struct {
	Entries                  uint32
	Flags                    uint32
	SQThreadCPU              uint32
	SQThreadIdle             uint32
	RegisterFixedBufferSize  uint32
	RegisterFixedBufferCount uint32
	PrepSQEBatchSize         uint32
	PrepSQEIdleTime          time.Duration
	PrepSQEAffCPU            int
	WaitCQEBatchSize         uint32
	WaitCQETimeCurve         Curve
	WaitCQEAffCPU            int
}

type Option func(*Options)

func WithEntries(entries uint32) Option {
	return func(opts *Options) {
		opts.Entries = entries
	}
}

func WithFlags(flags uint32) Option {
	return func(opts *Options) {
		opts.Flags = flags
	}
}

func WithSQThreadCPU(cpuId uint32) Option {
	return func(opts *Options) {
		opts.SQThreadCPU = cpuId
	}
}

func WithSQThreadIdle(idle uint32) Option {
	return func(opts *Options) {
		opts.SQThreadIdle = idle
	}
}

func WithPrepareSQEBatchSize(size uint32) Option {
	return func(opts *Options) {
		opts.PrepSQEBatchSize = size
	}
}

const (
	defaultPrepareSQEIdleTime = 15 * time.Second
)

func WithPrepareSQEIdleTime(d time.Duration) Option {
	return func(opts *Options) {
		if d < 1 {
			d = defaultPrepareSQEIdleTime
		}
		opts.PrepSQEIdleTime = d
	}
}

func WithPrepareSQEAFFCPU(cpu int) Option {
	return func(opts *Options) {
		opts.PrepSQEAffCPU = cpu
	}
}

func WithWaitCQEBatchSize(size uint32) Option {
	return func(opts *Options) {
		opts.WaitCQEBatchSize = size
	}
}

func WithWaitCQETimeCurve(curve Curve) Option {
	return func(opts *Options) {
		opts.WaitCQETimeCurve = curve
	}
}

func WithWaitCQEAFFCPU(cpu int) Option {
	return func(opts *Options) {
		opts.WaitCQEAffCPU = cpu
	}
}

func WithRegisterFixedBuffer(size uint32, count uint32) Option {
	return func(opts *Options) {
		if size == 0 || count == 0 {
			return
		}
		opts.RegisterFixedBufferSize = size
		opts.RegisterFixedBufferCount = count
	}
}

const (
	DefaultFlagsSchema     = "DEFAULT"
	PerformanceFlagsSchema = "PERFORMANCE"
)

func WithFlagsSchema(schema string) Option {
	return func(opts *Options) {
		if opts.Flags != 0 {
			return
		}
		schema = strings.TrimSpace(schema)
		schema = strings.ToUpper(schema)
		flags := uint32(0)
		switch schema {
		case DefaultFlagsSchema:
			flags = defaultIOURingSetupFlags()
			break
		case PerformanceFlagsSchema:
			flags = performanceIOURingSetupFlags()
			break
		default:
			flags = defaultIOURingSetupFlags()
			break
		}
		opts.Flags = flags
	}
}
