package async

type Promise[R any] interface {
	Complete(result R, err error)
	Succeed(result R)
	Fail(cause error)
	Future() (future Future[R])
}

func New[R any]() Promise[R] {
	return &promiseImpl[R]{
		ch: make(chan Result[R], 1),
	}
}

type promiseImpl[R any] struct {
	ch chan Result[R]
}

func (p *promiseImpl[R]) Complete(result R, err error) {
	if err == nil {
		p.Succeed(result)
	} else {
		p.Fail(err)
	}
}

func (p *promiseImpl[R]) Succeed(result R) {
	p.ch <- newAsyncResult[R](result, nil)
	close(p.ch)
}

func (p *promiseImpl[R]) Fail(cause error) {
	p.ch <- newAsyncResult[R](nil, cause)
	close(p.ch)
}

func (p *promiseImpl[R]) Future() (future Future[R]) {
	future = newFuture[R](p.ch)
	return
}
