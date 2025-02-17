package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/kernel"
	"runtime"
	"time"
)

type Options struct {
	CenterEntries     uint32
	SideEntries       []uint32
	SidesLoadBalancer LoadBalancer
	Flags             uint32
	Features          uint32
	WaitCQETimeout    time.Duration
	WaitCQEBatches    []uint32
}

type Option func(*Options) (err error)

func WithEntries(entries int) Option {
	return func(opts *Options) error {
		if entries > iouring.MaxEntries {
			return errors.New("entries too big")
		}
		if entries < 0 {
			entries = iouring.DefaultEntries
		}
		opts.CenterEntries = uint32(entries)
		return nil
	}
}

func WithSidesEntries(entries []int) Option {
	return func(opts *Options) error {
		sidesLen := len(entries)
		if sidesLen == 0 {
			return errors.New("sides must be greater than zero")
		}
		opts.SideEntries = make([]uint32, sidesLen)
		for i := 0; i < sidesLen; i++ {
			n := entries[i]
			if n > iouring.MaxEntries {
				return errors.New("one side entries too big")
			}
			if n < 0 {
				n = iouring.DefaultEntries
			}
			opts.SideEntries[i] = uint32(n)
		}
		return nil
	}
}

func WithSidesLoadBalancer(lb LoadBalancer) Option {
	return func(opts *Options) error {
		opts.SidesLoadBalancer = lb
		return nil
	}
}

func WithFlags(flags uint32) Option {
	return func(opts *Options) error {
		opts.Flags = flags
		return nil
	}
}

func WithFeatures(features uint32) Option {
	return func(opts *Options) error {
		opts.Features = features
		return nil
	}
}

func WithWaitCQETimeout(timeout time.Duration) Option {
	return func(opts *Options) error {
		if timeout < 1 {
			return errors.New("wait cqe timeout too small")
		}
		opts.WaitCQETimeout = timeout
		return nil
	}
}

func WithWaitCQEBatches(batches []uint32) Option {
	return func(opts *Options) error {
		batchesLen := len(batches)
		if batchesLen == 0 {
			return errors.New("wait cqe batches is empty")
		}
		for i := 0; i < batchesLen; i++ {
			if batches[i] < 1 {
				return errors.New("one if wait cqe batches is zero")
			}
		}
		opts.WaitCQEBatches = batches
		return nil
	}
}

const (
	minKernelVersionMajor = 5
	minKernelVersionMinor = 1
)

func New(options ...Option) (v *Vortexes, err error) {
	ver, verErr := kernel.GetKernelVersion()
	if verErr != nil {
		return nil, verErr
	}
	target := kernel.Version{
		Kernel: ver.Kernel,
		Major:  minKernelVersionMajor,
		Minor:  minKernelVersionMinor,
		Flavor: ver.Flavor,
	}

	if kernel.CompareKernelVersion(*ver, target) < 0 {
		return nil, errors.New("kernel version too low")
	}

	opt := Options{}
	for _, option := range options {
		if err = option(&opt); err != nil {
			return
		}
	}
	centerEntries := opt.CenterEntries
	if centerEntries == 0 {
		centerEntries = 8
	}
	sidesEntries := opt.SideEntries
	if len(sidesEntries) == 0 {
		cpuNum := runtime.NumCPU()
		sidesEntries = make([]uint32, cpuNum)
		for i := 0; i < cpuNum; i++ {
			sidesEntries[i] = iouring.DefaultEntries
		}
	}
	lb := opt.SidesLoadBalancer
	if lb == nil {
		lb = &RoundRobinLoadBalancer{}
	}
	flags := opt.Flags
	features := opt.Features
	if flags == 0 && features == 0 {
		flags, features = DefaultIOURingFlagsAndFeatures()
	}
	waitCQETimeout := opt.WaitCQETimeout
	if waitCQETimeout < 1 {
		waitCQETimeout = 50 * time.Millisecond
	}
	waitCQEBatches := opt.WaitCQEBatches
	if len(waitCQEBatches) == 0 {
		waitCQEBatches = []uint32{1, 2, 4, 8, 16, 32, 64, 96, 128, 256, 384, 512, 768, 1024, 1536, 2048, 3072, 4096, 5120, 6144, 7168, 8192, 10240}
	}

	center, centerErr := newVortex(centerEntries, flags, features, waitCQETimeout, waitCQEBatches)
	if centerErr != nil {
		err = centerErr
		return
	}
	sides := make([]*Vortex, len(sidesEntries))
	for i := 0; i < len(sides); i++ {
		sideEntries := sidesEntries[i]
		side, sideErr := newVortex(sideEntries, flags, features, waitCQETimeout, waitCQEBatches)
		if sideErr != nil {
			_ = center.Close()
			err = sideErr
			return
		}
		sides[i] = side
	}

	v = &Vortexes{
		center:            center,
		sides:             sides,
		sidesLoadBalancer: lb,
	}
	return
}

type Vortexes struct {
	center            *Vortex
	sides             []*Vortex
	sidesLoadBalancer LoadBalancer
}

func (vs *Vortexes) Start(ctx context.Context) {
	vs.center.start(ctx)
	for _, side := range vs.sides {
		side.start(ctx)
	}
}

func (vs *Vortexes) Center() *Vortex {
	return vs.center
}

func (vs *Vortexes) Side() *Vortex {
	n := vs.sidesLoadBalancer.Next(vs.sides)
	return vs.sides[n]
}

func (vs *Vortexes) Close() (err error) {
	for _, side := range vs.sides {
		if closeErr := side.Close(); closeErr != nil {
			if err == nil {
				err = closeErr
			} else {
				err = errors.Join(err, closeErr)
			}
		}
	}
	if closeErr := vs.center.Close(); closeErr != nil {
		if err == nil {
			err = closeErr
		} else {
			err = errors.Join(err, closeErr)
		}
	}
	return
}
