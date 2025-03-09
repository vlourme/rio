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
	UseCPUAffinity           bool
	SQAffinityCPU            uint32
	CQAffinityCPU            uint32
	RegisterFixedBufferSize  uint32
	RegisterFixedBufferCount uint32
	PrepareBatchSize         uint32
	PrepareIdleTime          time.Duration
	WaitTransmission         Transmission
}

type Option func(*Options)

func WithEntries(entries int) Option {
	return func(opts *Options) {
		opts.Entries = uint32(entries)
	}
}

func WithPrepareBatchSize(size uint32) Option {
	return func(opts *Options) {
		opts.PrepareBatchSize = size
	}
}

const (
	defaultPrepareIdleTime = 500 * time.Nanosecond
)

func WithPrepareIdleTime(d time.Duration) Option {
	return func(opts *Options) {
		if d < 1 {
			d = defaultPrepareIdleTime
		}
		opts.PrepareIdleTime = d
	}
}

func WithAffinityCPU(sqCPU int, cqCPU int) Option {
	return func(opts *Options) {
		if sqCPU < 0 || cqCPU < 0 {
			return
		}
		opts.UseCPUAffinity = true
		opts.SQAffinityCPU = uint32(sqCPU)
		opts.CQAffinityCPU = uint32(cqCPU)
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

func WithRegisterFixedBuffer(size uint32, count uint32) Option {
	return func(opts *Options) {
		if size == 0 || count == 0 {
			return
		}
		opts.RegisterFixedBufferSize = size
		opts.RegisterFixedBufferCount = count
	}
}

func WithWaitTransmission(transmission Transmission) Option {
	return func(opts *Options) {
		opts.WaitTransmission = transmission
	}
}

func WithCQEWaitTimeCurve(curve Curve) Option {
	return func(opts *Options) {
		opts.WaitTransmission = NewCurveTransmission(curve)
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
