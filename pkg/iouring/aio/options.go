package aio

type Options struct {
	Entries          uint32
	Flags            uint32
	SQThreadCPU      uint32
	SQThreadIdle     uint32
	PrepareBatchSize uint32
	UseCPUAffinity   bool
	WaitTransmission Transmission
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

func WithWaitTransmission(transmission Transmission) Option {
	return func(opts *Options) {
		opts.WaitTransmission = transmission
	}
}
