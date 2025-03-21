//go:build !linux

package rio

import (
	"github.com/brickingsoft/rio/pkg/iouring/aio"
)

// Presets
// preset aio options, must be called before Pin, Dial and Listen.
func Presets(_ ...aio.Option) {}
