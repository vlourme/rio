package aio

import (
	"syscall"
	"time"
)

type Transmission interface {
	Up() (uint32, *syscall.Timespec)
	Down() (uint32, *syscall.Timespec)
}

type Curve []struct {
	N       uint32
	Timeout time.Duration
}

var (
	defaultCurve = Curve{
		{1, 15 * time.Second},
		{8, 1 * time.Microsecond},
		{16, 10 * time.Microsecond},
		{32, 100 * time.Microsecond},
		{64, 200 * time.Microsecond},
		{96, 500 * time.Microsecond},
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
			timeout = 1 * time.Millisecond
		}
		times[i] = WaitNTime{
			n:    n,
			time: syscall.NsecToTimespec(timeout.Nanoseconds()),
		}
	}
	return &CurveTransmission{
		curve: times,
		size:  len(curve),
		idx:   -1,
	}
}

type WaitNTime struct {
	n    uint32
	time syscall.Timespec
}

type CurveTransmission struct {
	curve []WaitNTime
	size  int
	idx   int
}

func (tran *CurveTransmission) Up() (uint32, *syscall.Timespec) {
	if tran.idx == tran.size-1 {
		return tran.curve[tran.idx].n, &tran.curve[tran.idx].time
	}
	tran.idx++
	return tran.curve[tran.idx].n, &tran.curve[tran.idx].time
}

func (tran *CurveTransmission) Down() (uint32, *syscall.Timespec) {
	if tran.idx == 0 {
		return tran.curve[0].n, &tran.curve[0].time
	}
	tran.idx--
	return tran.curve[tran.idx].n, &tran.curve[tran.idx].time
}
