package aio

import (
	"syscall"
	"time"
)

type Transmission interface {
	Up() (uint32, *syscall.Timespec)
	Down() (uint32, *syscall.Timespec)
	Match(n uint32) (uint32, *syscall.Timespec)
}

type Curve []struct {
	N       uint32
	Timeout time.Duration
}

func NewCurveTransmission(curve Curve) Transmission {
	if len(curve) == 0 {
		curve = Curve{
			//{8, 5 * time.Microsecond},
			{16, 10 * time.Microsecond},
			{32, 20 * time.Microsecond},
			{64, 50 * time.Microsecond},
			//{96, 50 * time.Microsecond},
			//{128, 80 * time.Microsecond},
			//{192, 150 * time.Microsecond},
			//{256, 200 * time.Microsecond},
			//{384, 300 * time.Microsecond},
			//{512, 500 * time.Microsecond},
			//{8, 50 * time.Microsecond},
			//{16, 100 * time.Microsecond},
			//{32, 200 * time.Microsecond},
			//{96, 500 * time.Microsecond},
		}
	}
	times := make([]WaitNTime, 0, 1)
	for _, t := range curve {
		n := t.N
		if n < 1 || t.Timeout < 1 {
			continue
		}
		timeout := syscall.NsecToTimespec(t.Timeout.Nanoseconds())
		times = append(times, WaitNTime{
			n:    n,
			time: &timeout,
		})
	}
	return &CurveTransmission{
		curve: times,
		size:  len(curve),
		idx:   -1,
	}
}

type WaitNTime struct {
	n    uint32
	time *syscall.Timespec
}

type CurveTransmission struct {
	curve []WaitNTime
	size  int
	idx   int
}

func (tran *CurveTransmission) Up() (uint32, *syscall.Timespec) {
	if tran.idx == tran.size-1 {
		return tran.curve[tran.idx].n, tran.curve[tran.idx].time
	}
	tran.idx++
	return tran.curve[tran.idx].n, tran.curve[tran.idx].time
}

func (tran *CurveTransmission) Down() (uint32, *syscall.Timespec) {
	if tran.idx == 0 {
		return tran.curve[0].n, tran.curve[0].time
	}
	tran.idx--
	return tran.curve[tran.idx].n, tran.curve[tran.idx].time
}

func (tran *CurveTransmission) Match(n uint32) (uint32, *syscall.Timespec) {
	if n == 0 || tran.size == 1 {
		return tran.curve[0].n, tran.curve[0].time
	}
	left := WaitNTime{}
	for i := 0; i < tran.size; i++ {
		right := tran.curve[i]
		if left.n <= n && n < right.n {
			return right.n, right.time
		}
		left = right
	}
	tail := tran.curve[tran.size-1]
	return tail.n, tail.time
}
