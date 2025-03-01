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
	//defaultCurve = Curve{
	//	{1, 1 * time.Millisecond},
	//	{32, 1 * time.Millisecond},
	//	{64, 1 * time.Millisecond},
	//	{96, 1 * time.Millisecond},
	//	{128, 1 * time.Millisecond},
	//	{256, 1 * time.Millisecond},
	//	{384, 1 * time.Millisecond},
	//	{512, 1 * time.Millisecond},
	//	{768, 1 * time.Millisecond},
	//	{1024, 1 * time.Millisecond},
	//	{1536, 1 * time.Millisecond},
	//	{2048, 1 * time.Millisecond},
	//	{3072, 1 * time.Millisecond},
	//	{4096, 1 * time.Millisecond},
	//	{5120, 1 * time.Millisecond},
	//	{6144, 1 * time.Millisecond},
	//	{7168, 1 * time.Millisecond},
	//	{8192, 1 * time.Millisecond},
	//}
	defaultCurve = Curve{
		{1, 1 * time.Microsecond},
		{32, 10 * time.Microsecond},
		{64, 10 * time.Microsecond},
		{96, 20 * time.Microsecond},
		{128, 20 * time.Microsecond},
		{256, 20 * time.Microsecond},
		{384, 50 * time.Microsecond},
		{512, 50 * time.Microsecond},
		{768, 50 * time.Microsecond},
		{1024, 100 * time.Microsecond},
		{1536, 100 * time.Microsecond},
		{2048, 100 * time.Microsecond},
		{3072, 100 * time.Microsecond},
		{4096, 100 * time.Microsecond},
		{5120, 100 * time.Microsecond},
		{6144, 100 * time.Microsecond},
		{7168, 100 * time.Microsecond},
		{8192, 100 * time.Microsecond},
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
