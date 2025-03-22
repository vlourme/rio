//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/iouring"
)

func (op *Operation) PrepareNop() (err error) {
	op.kind = iouring.OpNop
	return
}

func (op *Operation) prepareLinkTimeout(target *Operation) {
	op.kind = iouring.OpLinkTimeout
	op.timeout = target.timeout
	target.linkTimeout = op
}
