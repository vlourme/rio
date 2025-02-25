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
	Entries           uint32
	Sides             uint32
	Flags             uint32
	Features          uint32
	WaitCQETimeout    time.Duration
	SidesLoadBalancer LoadBalancer
	WaitCQEBatches    []uint32
}

type Option func(*Options) (err error)

func WithEntries(entries int) Option {
	return func(opts *Options) error {
		if entries > iouring.MaxEntries {
			return errors.New("entries too big")
		}
		if entries < 1 {
			entries = iouring.DefaultEntries
		}
		opts.Entries = uint32(entries)
		return nil
	}
}

func WithSides(sides int) Option {
	return func(opts *Options) error {
		if sides < 1 {
			sides = runtime.NumCPU()
		}
		opts.Sides = uint32(sides)
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

func New(options ...Option) (v *Vortexes, err error) {
	opt := Options{}
	for _, option := range options {
		if err = option(&opt); err != nil {
			return
		}
	}
	entries := opt.Entries
	if entries == 0 {
		entries = iouring.DefaultEntries
	}
	sidesNum := opt.Sides
	if sidesNum < 1 {
		sidesNum = uint32(runtime.NumCPU()) - 1
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

	// center
	centerOptions := VortexOptions{
		Entries:        entries,
		Flags:          flags,
		Features:       features,
		WaitCQETimeout: waitCQETimeout,
		WaitCQEBatches: waitCQEBatches,
	}
	center, centerErr := NewVortex(centerOptions)
	if centerErr != nil {
		err = centerErr
		return
	}

	// sides
	var sides []*Vortex
	if sidesNum > 0 {
		sides = make([]*Vortex, sidesNum)
		for i := 0; i < len(sides); i++ {
			sideOptions := VortexOptions{
				Entries:        entries,
				Flags:          flags,
				Features:       features,
				WaitCQETimeout: waitCQETimeout,
				WaitCQEBatches: waitCQEBatches,
			}
			side, sideErr := NewVortex(sideOptions)
			if sideErr != nil {
				_ = center.Close()
				err = sideErr
				return
			}
			sides[i] = side
		}
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
	vs.center.Start(ctx)
	for _, side := range vs.sides {
		side.Start(ctx)
	}
}

func (vs *Vortexes) Center() *Vortex {
	return vs.center
}

func (vs *Vortexes) Side() *Vortex {
	n := vs.sidesLoadBalancer.Next(vs.sides)
	if n < 0 {
		return vs.center
	}
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

func CheckSendZCEnable() bool {
	ver, verErr := kernel.GetKernelVersion()
	if verErr != nil {
		return false
	}
	target := kernel.Version{
		Kernel: ver.Kernel,
		Major:  6,
		Minor:  0,
		Flavor: ver.Flavor,
	}
	if kernel.CompareKernelVersion(*ver, target) < 0 {
		return false
	}
	return true
}

func CheckSendMsdZCEnable() bool {
	ver, verErr := kernel.GetKernelVersion()
	if verErr != nil {
		return false
	}
	target := kernel.Version{
		Kernel: ver.Kernel,
		Major:  6,
		Minor:  1,
		Flavor: ver.Flavor,
	}
	if kernel.CompareKernelVersion(*ver, target) < 0 {
		return false
	}
	return true
}
