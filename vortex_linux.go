//go:build linux

package rio

import (
	"context"
	"errors"
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

func UseVortexNum(n int) {
	defaultVortexesOptions = append(
		defaultVortexesOptions,
		aio.WithN(n),
	)
}

func UseLoadBalancer(lb aio.LoadBalancer) {
	defaultVortexesOptions = append(
		defaultVortexesOptions,
		aio.WithLoadBalancer(lb),
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
func Pin() (err error) {
	if rv == nil {
		createReferencedVortexes()
	}
	err = rv.pin()
	return
}

// Unpin
// 不钉住 aio.Vortexes 。
func Unpin() (err error) {
	if rv != nil {
		err = rv.unpin()
		return
	}
	return errors.New("unpin failed cause not pinned")
}

var (
	rvp                     = atomic.Bool{}
	rv  *referencedVortexes = nil
)

func createReferencedVortexes() {
	for {
		if rvp.CompareAndSwap(false, true) {
			vortexes := aio.New(defaultVortexesOptions...)
			rv = &referencedVortexes{
				ref:      atomic.Int64{},
				vortexes: vortexes,
			}
			return
		}
		if rv != nil {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
	return
}

func getVortex() *aio.Vortex {
	return rv.vortex()
}

type referencedVortexes struct {
	ref      atomic.Int64
	vortexes *aio.Vortexes
}

func (v *referencedVortexes) vortex() *aio.Vortex {
	return v.vortexes.Vortex()
}

func (v *referencedVortexes) pin() (err error) {
	if n := v.ref.Add(1); n == 1 {
		ctx := context.Background()
		err = v.vortexes.Start(ctx)
	}
	return
}

func (v *referencedVortexes) unpin() (err error) {
	if n := v.ref.Add(-1); n < 1 {
		err = v.vortexes.Close()
	}
	return
}
