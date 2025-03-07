package aio

import (
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/kernel"
	"strings"
)

type Options struct {
	Entries                  uint32
	Flags                    uint32
	SQThreadCPU              uint32
	SQThreadIdle             uint32
	PrepareBatchSize         uint32
	UseCPUAffinity           bool
	RegisterFixedBufferSize  uint32
	RegisterFixedBufferCount uint32
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

func WithUseCPUAffinity(use bool) Option {
	return func(opts *Options) {
		opts.UseCPUAffinity = use
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

func WithCurveWaitTransmission(curve Curve) Option {
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

func defaultIOURingSetupFlags() uint32 {
	version, versionErr := kernel.Get()
	if versionErr != nil {
		return 0
	}
	major, minor := version.Major, version.Minor
	// flags
	flags := uint32(0)
	if compareKernelVersion(major, minor, 5, 18) >= 0 {
		// submit all
		flags |= iouring.SetupSubmitAll
	}
	return flags
}

func performanceIOURingSetupFlags() uint32 {
	version, versionErr := kernel.Get()
	if versionErr != nil {
		return 0
	}
	major, minor := version.Major, version.Minor
	// flags
	flags := uint32(0)
	if compareKernelVersion(major, minor, 5, 13) >= 0 {
		// submit all
		flags |= iouring.SetupSQPoll
		if compareKernelVersion(major, minor, 5, 18) >= 0 {
			// submit all
			flags |= iouring.SetupSubmitAll
			if compareKernelVersion(major, minor, 6, 0) >= 0 {
				// submit all
				flags |= iouring.SetupSingleIssuer
			}
		}
	}
	return flags
}
