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

// Pin
// usually used during program startup.
// such as only use Dial case, Pin before Dial and Unpin when program exit.
func Pin() error {
	return nil
}

// Unpin
// usually used during program exit.
func Unpin() error {
	return nil
}
