//go:build linux

package rio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/process"
	"sync"
)

// UseProcessPriority
// set process priority
func UseProcessPriority(level process.PriorityLevel) {
	_ = process.SetCurrentProcessPriority(level)
}

// Presets
// preset aio options, must be called before Pin, Dial and Listen.
func Presets(options ...aio.Option) {
	aio.Presets(options...)
}

// Pin
// usually used during program startup.
// such as only use Dial case, Pin before Dial and Unpin when program exit.
func Pin() error {
	pinOnce.Do(func() {
		pinned, pinErr = aio.Acquire()
	})
	return pinErr
}

// Unpin
// usually used during program exit.
func Unpin() error {
	unpinOnce.Do(func() {
		if pinned == nil {
			unpinErr = errors.New("unpin failed cause not pinned")
			return
		}
		unpinErr = aio.Release(pinned)
	})
	return unpinErr
}

var (
	pinned    *aio.Vortex = nil
	pinErr    error       = nil
	unpinErr  error       = nil
	pinOnce   sync.Once
	unpinOnce sync.Once
)
