package aio

import (
	"math/rand"
	"slices"
	"sync/atomic"
)

type LoadBalancer interface {
	Next(vs []*Vortex) (n int)
}

type RoundRobinLoadBalancer struct {
	pos atomic.Uint32
}

func (lb *RoundRobinLoadBalancer) Next(vs []*Vortex) (n int) {
	vsLen := uint32(len(vs))
	if vsLen == 0 {
		n = -1
		return
	}
	if vsLen == 1 {
		n = 0
		return
	}
	pos := lb.pos.Add(1)
	n = int(pos % vsLen)
	return
}

type RandomLoadBalancer struct{}

func (lb *RandomLoadBalancer) Next(vs []*Vortex) (n int) {
	vsLen := len(vs)
	if vsLen == 0 {
		n = -1
		return
	}
	if vsLen == 1 {
		n = 0
		return
	}
	return rand.Intn(vsLen)
}

type LeastLoadBalancer struct{}

func (lb *LeastLoadBalancer) Next(vs []*Vortex) (n int) {
	vsLen := len(vs)
	if vsLen == 0 {
		n = -1
		return
	}
	if vsLen == 1 {
		n = 0
		return
	}
	spaceLefts := make([]uint32, vsLen)

	for i := 0; i < vsLen; i++ {
		v := vs[i]
		sp := v.ring.SQSpaceLeft()
		spaceLefts[i] = sp
	}
	slices.Sort[[]uint32](spaceLefts)

	n = int(spaceLefts[vsLen-1])
	return
}
