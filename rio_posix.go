//go:build !linux

package rio

import (
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/process"
)

// UseProcessPriority
// set process priority
func UseProcessPriority(level process.PriorityLevel) {
	_ = process.SetCurrentProcessPriority(level)
}

// Presets
// preset aio options, must be called before Pin, Dial and Listen.
func Presets(_ ...aio.Option) {}
