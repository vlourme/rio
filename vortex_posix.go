//go:build !linux

package rio

import (
	"github.com/brickingsoft/rio/pkg/process"
)

func UseProcessPriority(level process.PriorityLevel) {
	_ = process.SetCurrentProcessPriority(level)
}

func Pin() error {
	return nil
}

func Unpin() error {
	return nil
}
