//go:build linux

package aio

import "github.com/brickingsoft/rio/pkg/kernel"

func CheckSendZCEnable() bool {
	ver, verErr := kernel.Get()
	if verErr != nil {
		return false
	}
	target := kernel.Version{
		Kernel: ver.Kernel,
		Major:  6,
		Minor:  0,
		Flavor: ver.Flavor,
	}
	if kernel.Compare(*ver, target) < 0 {
		return false
	}
	return true
}

func CheckSendMsdZCEnable() bool {
	ver, verErr := kernel.Get()
	if verErr != nil {
		return false
	}
	target := kernel.Version{
		Kernel: ver.Kernel,
		Major:  6,
		Minor:  1,
		Flavor: ver.Flavor,
	}
	if kernel.Compare(*ver, target) < 0 {
		return false
	}
	return true
}

const (
	minKernelVersionMajor = 5
	minKernelVersionMinor = 1
)

func compareKernelVersion(aMajor, aMinor, bMajor, bMinor int) int {
	if aMajor > bMajor {
		return 1
	} else if aMajor < bMajor {
		return -1
	}
	if aMinor > bMinor {
		return 1
	} else if aMinor < bMinor {
		return -1
	}
	return 0
}
