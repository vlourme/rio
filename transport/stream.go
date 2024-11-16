package transport

type Inbound interface {
	Reader() (buf InboundReader)
	Received() (n int)
}

func NewInbound(r InboundReader, n int) Inbound {
	return &inbound{
		r: r,
		n: n,
	}
}

type inbound struct {
	r InboundReader
	n int
}

func (in inbound) Reader() (r InboundReader) {
	r = in.r
	return
}

func (in inbound) Received() (n int) {
	n = in.n
	return
}

type Outbound interface {
	Wrote() (n int)
	UnexpectedError() (err error)
}

func NewOutBound(n int, err error) Outbound {
	return outbound{
		n:   n,
		err: err,
	}
}

type outbound struct {
	n   int
	err error
}

func (out outbound) UnexpectedError() (err error) {
	err = out.err
	return
}

func (out outbound) Wrote() (n int) {
	n = out.n
	return
}
