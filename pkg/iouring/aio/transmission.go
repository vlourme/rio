package aio

import (
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
	return &CurveTransmission{
		curve: curve,
		size:  len(curve),
	}
}

type CurveTransmission struct {
	curve Curve
	size  int
}

func (tran *CurveTransmission) Match(n uint32) syscall.Timespec {
	for i := 0; i < tran.size; i++ {
		p := tran.curve[i]
		if p.N >= n {
			p = tran.curve[i]
			return syscall.NsecToTimespec(p.Timeout.Nanoseconds())
		}
	}
	p := tran.curve[tran.size-1]
	return syscall.NsecToTimespec(p.Timeout.Nanoseconds())
}
