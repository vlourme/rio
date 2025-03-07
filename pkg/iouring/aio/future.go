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

func (f Future) Await(ctx context.Context) (n int, err error) {
	vortex := f.vortex
	op := f.op
	n, err = vortex.await(ctx, op)
	if op.borrowed {
		vortex.releaseOperation(op)
	}
	return
}

func (f Future) AwaitMsg(ctx context.Context) (n int, oobn int, flags int, addr unsafe.Pointer, addrLen uint32, err error) {
	vortex := f.vortex
	op := f.op
	n, err = vortex.await(ctx, op)
	if err == nil {
		oobn = int(op.msg.Controllen)
		flags = int(op.msg.Flags)
		addr = unsafe.Pointer(op.msg.Name)
		addrLen = op.msg.Namelen
	}
	if op.borrowed {
		vortex.releaseOperation(op)
	}
	return
}
