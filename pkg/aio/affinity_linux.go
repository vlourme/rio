//go:build linux

package aio

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"runtime"
)

func SetCPUAffinity(index int) error {
	newMask := unix.CPUSet{}
	newMask.Zero()

	cpuIndex := (index) % (runtime.NumCPU())
	newMask.Set(cpuIndex)

	pid := os.Getpid()
	err := unix.SchedSetaffinity(pid, &newMask)
	if err != nil {
		return fmt.Errorf("aio.SetCPUAffinity: %w, %v", err, newMask)
	}
	return nil
}
