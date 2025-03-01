package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/kernel"
	"runtime"
)

type Options struct {
	Entries                 uint32
	Sides                   uint32
	Flags                   uint32
	Features                uint32
	SidesLoadBalancer       LoadBalancer
	PrepareBatchSize        uint32
	WaitTransmissionBuilder TransmissionBuilder
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

func WithPrepareBatchSize(size uint32) Option {
	return func(opts *Options) error {
		opts.PrepareBatchSize = size
		return nil
	}
}

func WithSides(sides int) Option {
	return func(opts *Options) error {
		if sides < 1 {
			sides = 0
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

func WithWaitTransmissionBuilder(builder TransmissionBuilder) Option {
	return func(opts *Options) error {
		opts.WaitTransmissionBuilder = builder
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
		cpus := uint32(runtime.NumCPU())
		sidesNum = cpus / 2
		if sidesNum < 1 {
			sidesNum = 1
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

	prepareBatchSize := opt.PrepareBatchSize

	waitTransmissionBuilder := opt.WaitTransmissionBuilder
	if waitTransmissionBuilder == nil {
		waitTransmissionBuilder = NewCurveTransmissionBuilder(defaultCurve)
	}

	// center
	centerWaitTransmission, centerWaitTransmissionErr := waitTransmissionBuilder.Build()
	if centerWaitTransmissionErr != nil {
		err = centerWaitTransmissionErr
		return
	}
	centerOptions := VortexOptions{
		Entries:          entries,
		Flags:            flags,
		Features:         features,
		PrepareBatchSize: prepareBatchSize,
		WaitTransmission: centerWaitTransmission,
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
			sideWaitTransmission, sideWaitTransmissionErr := waitTransmissionBuilder.Build()
			if sideWaitTransmissionErr != nil {
				err = sideWaitTransmissionErr
				return
			}
			sideOptions := VortexOptions{
				Entries:          entries,
				Flags:            flags,
				Features:         features,
				WaitTransmission: sideWaitTransmission,
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
	ver, verErr := kernel.Get()
	if verErr != nil {
		return false
	}
	target := kernel.Version{
		Kernel: ver.Kernel,
		Major:  6,
		Minor:  0,
		Flavor: ver.Flavor,
	}
	if kernel.Compare(*ver, target) < 0 {
		return false
	}
	return true
}

func CheckSendMsdZCEnable() bool {
	ver, verErr := kernel.Get()
	if verErr != nil {
		return false
	}
	target := kernel.Version{
		Kernel: ver.Kernel,
		Major:  6,
		Minor:  1,
		Flavor: ver.Flavor,
	}
	if kernel.Compare(*ver, target) < 0 {
		return false
	}
	return true
}
