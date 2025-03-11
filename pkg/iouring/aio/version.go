package aio

import "github.com/brickingsoft/rio/pkg/kernel"

const (
	minKernelVersionMajor = 5
	minKernelVersionMinor = 1
)

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
