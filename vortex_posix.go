//go:build !linux

package rio

import (
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/process"
)

func UseProcessPriority(level process.PriorityLevel) {
	_ = process.SetCurrentProcessPriority(level)
}

func PrepareIOURingSetupOptions(_ ...aio.Option) {}

func Pin() error {
	return nil
}

func Unpin() error {
	return nil
}
