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
	Flags                   uint32
	Features                uint32
	UseCPUAffinity          bool
	PrepareBatchSize        uint32
	WaitTransmissionBuilder TransmissionBuilder
	Num                     int
	LoadBalancer            LoadBalancer
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

func WithCPUAffinity(use bool) Option {
	return func(opts *Options) {
		opts.UseCPUAffinity = use
	}
}

func WithN(n int) Option {
	return func(opts *Options) {
		opts.Num = n
	}
}

func WithLoadBalancer(lb LoadBalancer) Option {
	return func(opts *Options) {
		opts.LoadBalancer = lb
	}
}

func WithFlags(flags uint32) Option {
	return func(opts *Options) {
		opts.Flags = flags
	}
}

func WithFeatures(features uint32) Option {
	return func(opts *Options) {
		opts.Features = features
	}
}

func WithWaitTransmissionBuilder(builder TransmissionBuilder) Option {
	return func(opts *Options) {
		opts.WaitTransmissionBuilder = builder
	}
}

func New(options ...Option) (v *Vortexes) {
	opt := Options{
		Entries:                 iouring.DefaultEntries,
		Flags:                   0,
		Features:                0,
		UseCPUAffinity:          false,
		PrepareBatchSize:        0,
		WaitTransmissionBuilder: nil,
		Num:                     -1,
		LoadBalancer:            nil,
	}
	for _, option := range options {
		option(&opt)
	}
	entries := opt.Entries
	if entries == 0 {
		entries = iouring.DefaultEntries
	}
	flags := opt.Flags
	features := opt.Features
	if flags == 0 && features == 0 {
		flags, features = DefaultIOURingFlagsAndFeatures()
	}
	prepareBatchSize := opt.PrepareBatchSize
	useCPUAffinity := opt.UseCPUAffinity
	waitTransmissionBuilder := opt.WaitTransmissionBuilder
	if waitTransmissionBuilder == nil {
		waitTransmissionBuilder = NewCurveTransmissionBuilder(defaultCurve)
	}

	num := opt.Num
	if num < 1 {
		cpus := runtime.NumCPU()
		num = cpus / 8
		if num < 1 {
			num = 1
		}
	}
	lb := opt.LoadBalancer
	if lb == nil {
		lb = &RoundRobinLoadBalancer{}
	}
	vortexes := make([]*Vortex, num)
	for i := 0; i < num; i++ {
		waitTransmission := waitTransmissionBuilder.Build()
		vortexOptions := VortexOptions{
			Entries:          entries,
			Flags:            flags,
			Features:         features,
			PrepareBatchSize: prepareBatchSize,
			UseCPUAffinity:   useCPUAffinity,
			WaitTransmission: waitTransmission,
		}
		vortex := NewVortex(vortexOptions)
		vortex.SetId(i)
		vortexes[i] = vortex
	}

	v = &Vortexes{
		vortexes:     vortexes,
		vortexesLen:  num,
		loadBalancer: lb,
	}
	return
}

type Vortexes struct {
	vortexes     []*Vortex
	vortexesLen  int
	loadBalancer LoadBalancer
}

func (vs *Vortexes) Start(ctx context.Context) (err error) {
	for _, vortex := range vs.vortexes {
		if err = vortex.Start(ctx); err != nil {
			_ = vs.Close()
			return
		}
	}
	return
}

func (vs *Vortexes) Vortex() *Vortex {
	if vs.vortexesLen == 0 {
		return nil
	}
	if vs.vortexesLen == 1 {
		return vs.vortexes[0]
	}
	n := vs.loadBalancer.Next(vs.vortexes)
	if n < 0 {
		return nil
	}
	return vs.vortexes[n]
}

func (vs *Vortexes) Close() (err error) {
	for _, vortex := range vs.vortexes {
		if closeErr := vortex.Close(); closeErr != nil {
			if err == nil {
				err = closeErr
			} else {
				err = errors.Join(err, closeErr)
			}
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
