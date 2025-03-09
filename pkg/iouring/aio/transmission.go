package aio

import (
	"sort"
	"syscall"
	"time"
)

type Transmission interface {
	Match(n uint32) syscall.Timespec
}

type Curve []struct {
	N       uint32
	Timeout time.Duration
}

var (
	defaultCurve = Curve{
		{1, 500 * time.Nanosecond},
		{8, 1 * time.Microsecond},
		{32, 2 * time.Microsecond},
		{64, 4 * time.Microsecond},
		{96, 8 * time.Microsecond},
		{128, 12 * time.Microsecond},
		{256, 16 * time.Microsecond},
		{512, 32 * time.Microsecond},
		{1024, 64 * time.Microsecond},
		{2048, 128 * time.Microsecond},
		{4096, 256 * time.Microsecond},
		{8192, 512 * time.Microsecond},
		{10240, 1000 * time.Microsecond},
	}
)

func NewCurveTransmission(curve Curve) Transmission {
	if len(curve) == 0 {
		curve = defaultCurve
	}
	times := make([]WaitNTime, len(curve))
	for i, t := range curve {
		n := t.N
		if n == 0 {
			n = 1
		}
		timeout := t.Timeout
		if timeout < 1 {
			timeout = defaultWaitTimeout
		}
		times[i] = WaitNTime{
			N:    n,
			time: syscall.NsecToTimespec(timeout.Nanoseconds()),
		}
	}
	sort.Slice(times, func(i, j int) bool {
		return times[i].N < times[j].N
	})
	return &CurveTransmission{
		curve: times,
		size:  len(curve),
	}
}

type WaitNTime struct {
	N    uint32
	time syscall.Timespec
}

type CurveTransmission struct {
	curve []WaitNTime
	size  int
}

func (tran *CurveTransmission) Match(n uint32) syscall.Timespec {
	for i := 0; i < tran.size; i++ {
		p := tran.curve[i]
		if p.N >= n {
			p = tran.curve[i]
			return p.time
		}
	}
	p := tran.curve[tran.size-1]
	return p.time
}
