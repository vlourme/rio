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
	RegisterFixedFiles       uint32
	PrepSQEBatchSize         uint32
	PrepSQEBatchTimeWindow   time.Duration
	PrepSQEBatchIdleTime     time.Duration
	PrepSQEBatchAffCPU       int
	WaitCQEBatchSize         uint32
	WaitCQEBatchTimeCurve    Curve
	WaitCQEBatchAffCPU       int
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
	defaultPrepSQEBatchTimeWindow = 500 * time.Nanosecond
)

func WithPrepSQEBatchTimeWindow(window time.Duration) Option {
	return func(opts *Options) {
		if window < 1 {
			window = defaultPrepSQEBatchTimeWindow
		}
		opts.PrepSQEBatchTimeWindow = window
	}
}

const (
	defaultPrepSQEBatchIdleTime = 15 * time.Second
)

func WithPrepareSQEBatchIdleTime(d time.Duration) Option {
	return func(opts *Options) {
		if d < 1 {
			d = defaultPrepSQEBatchIdleTime
		}
		opts.PrepSQEBatchIdleTime = d
	}
}

func WithPrepareSQEBatchAFFCPU(cpu int) Option {
	return func(opts *Options) {
		opts.PrepSQEBatchAffCPU = cpu
	}
}

func WithWaitCQEBatchSize(size uint32) Option {
	return func(opts *Options) {
		opts.WaitCQEBatchSize = size
	}
}

func WithWaitCQEBatchTimeCurve(curve Curve) Option {
	return func(opts *Options) {
		opts.WaitCQEBatchTimeCurve = curve
	}
}

func WithWaitCQEBatchAFFCPU(cpu int) Option {
	return func(opts *Options) {
		opts.WaitCQEBatchAffCPU = cpu
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

func WithRegisterFixedFiles(files uint32) Option {
	return func(opts *Options) {
		opts.RegisterFixedFiles = files
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
