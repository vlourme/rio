package aio

import (
	"time"
)

type Transmission interface {
	Up() (uint32, time.Duration)
	Down() (uint32, time.Duration)
	Match(n uint32) (uint32, time.Duration)
}

type Curve []struct {
	N       uint32
	Timeout time.Duration
}

func NewCurveTransmission(curve Curve) Transmission {
	if len(curve) == 0 {
		curve = Curve{
			{8, 1 * time.Microsecond},
			{16, 10 * time.Microsecond},
			{32, 200 * time.Microsecond},
			{64, 300 * time.Microsecond},
			{96, 500 * time.Microsecond},
		}
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
			time: timeout,
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
	time time.Duration
}

type CurveTransmission struct {
	curve []WaitNTime
	size  int
	idx   int
}

func (tran *CurveTransmission) Up() (uint32, time.Duration) {
	if tran.idx == tran.size-1 {
		return tran.curve[tran.idx].n, tran.curve[tran.idx].time
	}
	tran.idx++
	return tran.curve[tran.idx].n, tran.curve[tran.idx].time
}

func (tran *CurveTransmission) Down() (uint32, time.Duration) {
	if tran.idx == 0 {
		return tran.curve[0].n, tran.curve[0].time
	}
	tran.idx--
	return tran.curve[tran.idx].n, tran.curve[tran.idx].time
}

func (tran *CurveTransmission) Match(n uint32) (uint32, time.Duration) {
	head := tran.curve[0]
	if n < head.n {
		return 0, 0
	}
	for i := range tran.curve {
		node := tran.curve[i]
		if node.n >= n {
			return node.n, node.time
		}
	}
	tail := tran.curve[len(tran.curve)-1]
	return tail.n, tail.time
}
