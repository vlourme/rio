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
	Prev() (n uint32, timeout time.Duration)
	Next() (n uint32, timeout time.Duration)
	Close() error
}

type Curve []struct {
	N       uint32
	Timeout time.Duration
}

var (
	defaultCurve = Curve{
		{1, 10 * time.Microsecond},
		{2, 20 * time.Microsecond},
		{4, 30 * time.Microsecond},
		{8, 80 * time.Microsecond},
		{16, 160 * time.Microsecond},
		{32, 320 * time.Microsecond},
		{64, 640 * time.Microsecond},
		{96, 960 * time.Microsecond},
		{128, 1280 * time.Microsecond},
		{256, 2560 * time.Microsecond},
		{384, 3840 * time.Microsecond},
		{512, 5120 * time.Microsecond},
		{768, 7680 * time.Microsecond},
		{1024, 10240 * time.Microsecond},
		{1536, 15360 * time.Microsecond},
		{2048, 20480 * time.Microsecond},
		{3072, 30720 * time.Microsecond},
		{4096, 40960 * time.Microsecond},
		{5120, 51200 * time.Microsecond},
		{6144, 61440 * time.Microsecond},
		{7168, 71680 * time.Microsecond},
		{8192, 81920 * time.Microsecond},
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

func (tran *CurveTransmission) Prev() (n uint32, timeout time.Duration) {
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

func (tran *CurveTransmission) Next() (n uint32, timeout time.Duration) {
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
