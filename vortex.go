package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"sync"
	"sync/atomic"
	"time"
)

var (
	defaultUseSendZC       = atomic.Bool{}
	defaultVortexesOptions []aio.Option
)

func UseZeroCopy(use bool) {
	defaultUseSendZC.Store(use)
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

func UseWaitCQETimeout(timeout time.Duration) {
	defaultVortexesOptions = append(
		defaultVortexesOptions,
		aio.WithWaitCQETimeout(timeout),
	)
}

func UseWaitCQEBatches(batches []uint32) {
	defaultVortexesOptions = append(
		defaultVortexesOptions,
		aio.WithWaitCQEBatches(batches),
	)
}

var (
	rvLocker                     = new(sync.Mutex)
	rv       *referencedVortexes = nil
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
	rv.start()
	return
}

func getCenterVortex() (*aio.Vortex, error) {
	rvLocker.Lock()
	defer rvLocker.Unlock()
	if rv == nil {
		if err := createReferencedVortexes(); err != nil {
			return nil, err
		}
	}
	return rv.center(), nil
}

func getSideVortex() (*aio.Vortex, error) {
	rvLocker.Lock()
	defer rvLocker.Unlock()
	if rv == nil {
		if err := createReferencedVortexes(); err != nil {
			return nil, err
		}
	}
	return rv.side(), nil
}

// PinVortexes
// 钉住 aio.Vortexes 。
// 一般用于程序启动时。
// 这用手动管理 aio.Vortexes 的生命周期，一般用于只有 Dial 的使用。
// 注意：必须 UnpinVortexes 来关闭 aio.Vortexes 。
func PinVortexes() (err error) {
	rvLocker.Lock()
	defer rvLocker.Unlock()
	if rv == nil {
		if err = createReferencedVortexes(); err != nil {
			return
		}
	}
	rv.pin()
	return
}

// UnpinVortexes
// 不钉住 aio.Vortexes 。
func UnpinVortexes() (err error) {
	rvLocker.Lock()
	defer rvLocker.Unlock()
	if rv == nil {
		return
	}
	rv.unpin()
	ok := false
	ok, err = rv.tryClose()
	if ok {
		rv = nil
	}
	return
}

type referencedVortexes struct {
	ref      atomic.Int64
	vortexes *aio.Vortexes
}

func (v *referencedVortexes) start() {
	ctx := context.Background()
	v.vortexes.Start(ctx)
}

func (v *referencedVortexes) center() *aio.Vortex {
	v.pin()
	return v.vortexes.Center()
}

func (v *referencedVortexes) side() *aio.Vortex {
	return v.vortexes.Side()
}

func (v *referencedVortexes) pin() {
	v.ref.Add(1)
}

func (v *referencedVortexes) unpin() {
	v.ref.Add(-1)
}

func (v *referencedVortexes) tryClose() (bool, error) {
	if v.ref.Load() == 0 {
		return true, v.vortexes.Close()
	}
	return false, nil
}
