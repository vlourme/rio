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
