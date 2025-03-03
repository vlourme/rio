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
		{2, 2 * time.Microsecond},
		{4, 4 * time.Microsecond},
		{6, 6 * time.Microsecond},
		{8, 8 * time.Microsecond},
		{16, 16 * time.Microsecond},
		{24, 24 * time.Microsecond},
		{32, 32 * time.Microsecond},
		{64, 64 * time.Microsecond},
		{96, 96 * time.Microsecond},
		{128, 128 * time.Microsecond},
		{256, 512 * time.Microsecond},
		{384, 384 * time.Microsecond},
		{512, 512 * time.Microsecond},
		{768, 768 * time.Microsecond},
		{1024, 1024 * time.Microsecond},
		{1536, 1536 * time.Microsecond},
		{2048, 2048 * time.Microsecond},
		{3072, 3072 * time.Microsecond},
		{4096, 4096 * time.Microsecond},
		{5120, 5120 * time.Microsecond},
		{6144, 6144 * time.Microsecond},
		{7168, 7168 * time.Microsecond},
		{8192, 8192 * time.Microsecond},
		{10240, 10240 * time.Microsecond},
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
			tran.idx = i
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
