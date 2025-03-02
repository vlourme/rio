package aio

import "time"

type TransmissionBuilder interface {
	Build() (Transmission, error)
}

func NewCurveTransmissionBuilder(curve Curve) TransmissionBuilder {
	if len(curve) == 0 {
		curve = defaultCurve
	}
	return &CurveTransmissionBuilder{
		curve: curve,
	}
}

type CurveTransmissionBuilder struct {
	curve Curve
}

func (builder *CurveTransmissionBuilder) Build() (Transmission, error) {
	return NewCurveTransmission(builder.curve), nil
}

type Transmission interface {
	MatchN(n uint32) (uint32, time.Duration)
	Up() (n uint32, timeout time.Duration)
	Down() (n uint32, timeout time.Duration)
	Close() error
}

type Curve []struct {
	N       uint32
	Timeout time.Duration
}

var (
	defaultCurve = Curve{
		{1, 1 * time.Microsecond},
		{32, 1 * time.Microsecond},
		{64, 1 * time.Microsecond},
		{96, 1 * time.Microsecond},
		{128, 5 * time.Microsecond},
		{256, 10 * time.Microsecond},
		{384, 15 * time.Microsecond},
		{512, 20 * time.Microsecond},
		{768, 25 * time.Microsecond},
		{1024, 30 * time.Microsecond},
		{1536, 30 * time.Microsecond},
		{2048, 40 * time.Microsecond},
		{3072, 40 * time.Microsecond},
		{4096, 40 * time.Microsecond},
		{5120, 50 * time.Microsecond},
		{6144, 50 * time.Microsecond},
		{7168, 60 * time.Microsecond},
		{8192, 60 * time.Microsecond},
		{10240, 100 * time.Microsecond},
	}
)

func NewCurveTransmission(curve Curve) Transmission {
	if len(curve) == 0 {
		curve = defaultCurve
	}
	return &CurveTransmission{
		curve: curve,
		idx:   -1,
		size:  len(curve),
	}
}

type CurveTransmission struct {
	curve Curve
	idx   int
	size  int
}

func (tran *CurveTransmission) MatchN(n uint32) (uint32, time.Duration) {
	for i := 1; i < tran.size; i++ {
		p := tran.curve[i]
		if p.N > n {
			tran.idx = i - 1
			p = tran.curve[tran.idx]
			return p.N, p.Timeout
		}
	}
	tran.idx = tran.size - 1
	p := tran.curve[tran.idx]
	return p.N, p.Timeout
}

func (tran *CurveTransmission) Down() (n uint32, timeout time.Duration) {
	if tran == nil || tran.size == 0 {
		return
	}
	tran.idx--
	if tran.idx < 0 {
		tran.idx = 0
	}
	idx := tran.idx % tran.size
	node := tran.curve[idx]
	n, timeout = node.N, node.Timeout
	return
}

func (tran *CurveTransmission) Up() (n uint32, timeout time.Duration) {
	if tran == nil || tran.size == 0 {
		return
	}
	tran.idx++
	if tran.idx < 0 {
		tran.idx = 0
	}
	idx := tran.idx % tran.size
	node := tran.curve[idx]
	n, timeout = node.N, node.Timeout
	return
}

func (tran *CurveTransmission) Close() error {
	return nil
}
