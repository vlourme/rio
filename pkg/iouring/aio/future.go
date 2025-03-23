//go:build linux

package aio

import (
	"context"
	"syscall"
)

type Future struct {
	vortex *Vortex
	op     *Operation
}

func (f *Future) Operation() *Operation {
	return f.op
}

func (f *Future) Await(ctx context.Context) (n int, cqeFlags uint32, err error) {
	op := f.op
	n, cqeFlags, err = f.vortex.awaitOperation(ctx, op)
	f.vortex.releaseOperation(op)
	return
}

type AcceptFuture struct {
	vortex      *Vortex
	op          *Operation
	ln          *NetFd
	addr        *syscall.RawSockaddrAny
	addrLen     *int
	directAlloc bool
}

func (f *AcceptFuture) Operation() *Operation {
	return f.op
}

func (f *AcceptFuture) Await(ctx context.Context) (fd *NetFd, cqeFlags uint32, err error) {
	var (
		op          = f.op
		ln          = f.ln
		accepted    = -1
		directAlloc = f.directAlloc
	)
	accepted, cqeFlags, err = f.vortex.awaitOperation(ctx, op)
	f.vortex.releaseOperation(op)
	if err != nil {
		return
	}
	fd, err = newAcceptedNetFd(ln, accepted, directAlloc)
	return
}
