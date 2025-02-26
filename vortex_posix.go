//go:build !linux

package rio

import (
	"github.com/brickingsoft/rio/pkg/process"
)

func UseProcessPriority(level process.PriorityLevel) {
	_ = process.SetCurrentProcessPriority(level)
}

func PinVortexes() error {
	return nil
}

func UnpinVortexes() error {
	return nil
}
