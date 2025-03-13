//go:build linux

package aio

import (
	"context"
	"unsafe"
)

type Future struct {
	vortex *Vortex
	op     *Operation
}

func (f *Future) Operation() *Operation {
	return f.op
}

func (f *Future) Await(ctx context.Context) (n int, cqeFlags uint32, err error) {
	vortex := f.vortex
	op := f.op
	n, cqeFlags, err = vortex.AwaitOperation(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (f *Future) AwaitMsg(ctx context.Context) (n int, oobn int, flags int, addr unsafe.Pointer, addrLen uint32, cqeFlags uint32, err error) {
	vortex := f.vortex
	op := f.op
	n, cqeFlags, err = vortex.AwaitOperation(ctx, op)
	if err == nil {
		oobn = int(op.msg.Controllen)
		flags = int(op.msg.Flags)
		addr = unsafe.Pointer(op.msg.Name)
		addrLen = op.msg.Namelen
	}
	vortex.releaseOperation(op)
	return
}
