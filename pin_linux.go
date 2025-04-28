//go:build linux

package rio

import (
	"github.com/brickingsoft/rio/pkg/liburing/aio"
)

// Pin iouring ctx, use for dial only or multi listen.
func Pin() {
	_ = aio.Pin()
}

// Unpin iouring ctx
func Unpin() {
	aio.Unpin()
}
