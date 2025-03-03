//go:build linux

package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/process"
	"sync/atomic"
	"time"
)

var (
	defaultVortexesOptions []aio.Option
)

func UseProcessPriority(level process.PriorityLevel) {
	_ = process.SetCurrentProcessPriority(level)
}

func UsePreformMode() {
	flags, feats := aio.PerformIOURingFlagsAndFeatures()
	defaultVortexesOptions = append(
		defaultVortexesOptions,
		aio.WithFlags(flags),
		aio.WithFeatures(feats),
	)
}

func UseFlags(flags uint32) {
	defaultVortexesOptions = append(
		defaultVortexesOptions,
		aio.WithFlags(flags),
	)
}

func UseFeatures(feats uint32) {
	defaultVortexesOptions = append(
		defaultVortexesOptions,
		aio.WithFeatures(feats),
	)
}

func UseEntries(entries int) {
	defaultVortexesOptions = append(
		defaultVortexesOptions,
		aio.WithEntries(entries),
	)
}

func UsePrepareBatchSize(size uint32) {
	defaultVortexesOptions = append(
		defaultVortexesOptions,
		aio.WithPrepareBatchSize(size),
	)
}

func UseSides(sides int) {
	defaultVortexesOptions = append(
		defaultVortexesOptions,
		aio.WithSides(sides),
	)
}

func UseSidesLoadBalancer(lb aio.LoadBalancer) {
	defaultVortexesOptions = append(
		defaultVortexesOptions,
		aio.WithSidesLoadBalancer(lb),
	)
}

func UseWaitTransmissionBuilder(builder aio.TransmissionBuilder) {
	defaultVortexesOptions = append(
		defaultVortexesOptions,
		aio.WithWaitTransmissionBuilder(builder),
	)
}

func UseCPUAffinity(use bool) {
	defaultVortexesOptions = append(
		defaultVortexesOptions,
		aio.WithCPUAffinity(use),
	)
}

// Pin
// 钉住 aio.Vortexes 。
// 一般用于程序启动时。
// 这用手动管理 aio.Vortexes 的生命周期，一般用于只有 Dial 的使用。
// 注意：必须 Unpin 来关闭 aio.Vortexes 。
func Pin() {
	if rv == nil {
		if err := createReferencedVortexes(); err != nil {
			panic(err)
			return
		}
	}
	rv.pin()
	return
}

// Unpin
// 不钉住 aio.Vortexes 。
func Unpin() {
	if rv != nil {
		rv.unpin()
	}
	return
}

var (
	rvp                     = atomic.Bool{}
	rv  *referencedVortexes = nil
)

func createReferencedVortexes() (err error) {
	for {
		if rvp.CompareAndSwap(false, true) {
			vortexes, vortexesErr := aio.New(defaultVortexesOptions...)
			if vortexesErr != nil {
				err = vortexesErr
				return
			}
			rv = &referencedVortexes{
				ref:      atomic.Int64{},
				vortexes: vortexes,
			}
			return
		}
		if rv != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	return
}

func getCenterVortex() (*aio.Vortex, error) {
	if rv == nil {
		if err := createReferencedVortexes(); err != nil {
			return nil, err
		}
	}
	rv.pin()
	return rv.center(), nil
}

func getSideVortex() *aio.Vortex {
	return rv.side()
}

type referencedVortexes struct {
	ref      atomic.Int64
	vortexes *aio.Vortexes
}

func (v *referencedVortexes) center() *aio.Vortex {
	return v.vortexes.Center()
}

func (v *referencedVortexes) side() *aio.Vortex {
	return v.vortexes.Side()
}

func (v *referencedVortexes) pin() {
	if n := v.ref.Add(1); n == 1 {
		ctx := context.Background()
		v.vortexes.Start(ctx)
	}
}

func (v *referencedVortexes) unpin() {
	if n := v.ref.Add(-1); n < 1 {
		_ = v.vortexes.Close()
	}
}
