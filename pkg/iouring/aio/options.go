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
	PrepareSQBatchSize       uint32
	PrepareSQAffinityCPU     uint32
	PrepareSQIdleTime        time.Duration
	WaitCQBatchSize          uint32
	WaitCQAffinityCPU        uint32
	WaitCQTransmission       Transmission
}

type Option func(*Options)

func WithEntries(entries int) Option {
	return func(opts *Options) {
		opts.Entries = uint32(entries)
	}
}

func WithPrepareSQBatchSize(size uint32) Option {
	return func(opts *Options) {
		opts.PrepareSQBatchSize = size
	}
}

const (
	defaultPrepareSQIdleTime = 500 * time.Nanosecond
)

func WithPrepareSQIdleTime(d time.Duration) Option {
	return func(opts *Options) {
		if d < 1 {
			d = defaultPrepareSQIdleTime
		}
		opts.PrepareSQIdleTime = d
	}
}

func WithPrepareSQAffinityCPU(cpuId uint32) Option {
	return func(opts *Options) {
		opts.PrepareSQAffinityCPU = cpuId
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

func WithWaitCQBatchSize(size uint32) Option {
	return func(opts *Options) {
		opts.WaitCQBatchSize = size
	}
}

func WithWaitCQAffinityCPU(cpuId uint32) Option {
	return func(opts *Options) {
		opts.WaitCQAffinityCPU = cpuId
	}
}

func WithWaitCQTransmission(transmission Transmission) Option {
	return func(opts *Options) {
		opts.WaitCQTransmission = transmission
	}
}

func WithWaitCQTimeCurve(curve Curve) Option {
	return func(opts *Options) {
		opts.WaitCQTransmission = NewCurveTransmission(curve)
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
