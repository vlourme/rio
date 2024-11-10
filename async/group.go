package async

func Group[R any](promises []Promise[R]) (future Future[R]) {
	group := groupFuture[R]{
		members: make([]Future[R], len(promises)),
	}
	for i, promise := range promises {
		group.members[i] = promise.Future()
	}
	future = &group
	return
}

type groupFuture[R any] struct {
	members []Future[R]
}

func (group *groupFuture[R]) OnComplete(handler ResultHandler[R]) {
	for _, member := range group.members {
		member.OnComplete(handler)
	}
}
