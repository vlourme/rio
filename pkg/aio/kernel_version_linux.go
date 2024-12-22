//go:build linux

package aio

import (
	"golang.org/x/sys/unix"
	"sync"
)

var (
	kernelVersionMajor = 0
	kernelVersionMinor = 0
	kernelVersionOnce  = sync.Once{}
)

func KernelVersion() (major, minor int) {
	kernelVersionOnce.Do(func() {
		var uname unix.Utsname
		if err := unix.Uname(&uname); err != nil {
			return
		}
		var (
			values    [2]int
			value, vi int
		)
		for _, c := range uname.Release {
			if '0' <= c && c <= '9' {
				value = (value * 10) + int(c-'0')
			} else {
				values[vi] = value
				vi++
				if vi >= len(values) {
					break
				}
				value = 0
			}
		}
		kernelVersionMajor = values[0]
		kernelVersionMinor = values[1]
	})
	return kernelVersionMajor, kernelVersionMinor
}
