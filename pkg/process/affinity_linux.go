//go:build linux

package process

import (
	"fmt"
	"golang.org/x/sys/unix"
	"runtime"
)

func SetCPUAffinity(index int) error {
	var newMask unix.CPUSet

	newMask.Zero()

	cpuIndex := (index) % (runtime.NumCPU())
	newMask.Set(cpuIndex)

	err := unix.SchedSetaffinity(0, &newMask)
	if err != nil {
		return fmt.Errorf("SchedSetaffinity: %w, %v", err, newMask)
	}

	return nil
}
