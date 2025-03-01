//go:build linux

package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/process"
	"sync"
	"sync/atomic"
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

// PinVortexes
// 钉住 aio.Vortexes 。
// 一般用于程序启动时。
// 这用手动管理 aio.Vortexes 的生命周期，一般用于只有 Dial 的使用。
// 注意：必须 UnpinVortexes 来关闭 aio.Vortexes 。
func PinVortexes() (err error) {
	rvOnce.Do(func() {
		err = createReferencedVortexes()
	})
	if err != nil {
		return
	}
	rv.pin()
	return
}

// UnpinVortexes
// 不钉住 aio.Vortexes 。
func UnpinVortexes() (err error) {
	rvOnce.Do(func() {
		err = createReferencedVortexes()
	})
	if err != nil {
		return
	}
	rv.unpin()
	return
}

var (
	rvOnce                     = new(sync.Once)
	rv     *referencedVortexes = nil
)

func createReferencedVortexes() (err error) {
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

func getCenterVortex() (*aio.Vortex, error) {
	err := PinVortexes()
	if err != nil {
		return nil, err
	}
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
	v.pin()
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
