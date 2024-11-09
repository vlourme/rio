package transport

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
