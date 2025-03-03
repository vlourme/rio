//go:build !linux

package rio

import (
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/process"
)

func UseProcessPriority(level process.PriorityLevel) {
	_ = process.SetCurrentProcessPriority(level)
}

func UsePreformMode() {}

func UseFlags(flags uint32) {}

func UseFeatures(feats uint32) {}

func UseEntries(entries int) {}

func UsePrepareBatchSize(size uint32) {}

func UseVortexNum(n int) {}

func UseLoadBalancer(lb aio.LoadBalancer) {}

func UseWaitTransmissionBuilder(builder aio.TransmissionBuilder) {}

func UseCPUAffinity(use bool) {}

func Pin() error {
	return nil
}

func Unpin() error {
	return nil
}
