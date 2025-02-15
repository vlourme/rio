package iouring

import (
	"fmt"

	"golang.org/x/sys/unix"
)

type KernelVersion struct {
	Kernel int
	Major  int
	Minor  int
	Flavor string
}

const (
	firstNumberOfParts  = 2
	secondNumberOfParts = 1
)

func parseKernelVersion(kernelVersionStr string) (*KernelVersion, error) {
	var (
		kernel, major, minor, parsed int
		flavor, partial              string
	)

	parsed, _ = fmt.Sscanf(kernelVersionStr, "%d.%d%s", &kernel, &major, &partial)
	if parsed < firstNumberOfParts {
		return nil, fmt.Errorf("cannot parse kernel version: %s", kernelVersionStr)
	}

	parsed, _ = fmt.Sscanf(partial, ".%d%s", &minor, &flavor)
	if parsed < secondNumberOfParts {
		flavor = partial
	}

	return &KernelVersion{
		Kernel: kernel,
		Major:  major,
		Minor:  minor,
		Flavor: flavor,
	}, nil
}

func GetKernelVersion() (*KernelVersion, error) {
	uts := &unix.Utsname{}

	if err := unix.Uname(uts); err != nil {
		return nil, err
	}

	return parseKernelVersion(unix.ByteSliceToString(uts.Release[:]))
}

func CompareKernelVersion(a, b KernelVersion) int {
	if a.Kernel > b.Kernel {
		return 1
	} else if a.Kernel < b.Kernel {
		return -1
	}

	if a.Major > b.Major {
		return 1
	} else if a.Major < b.Major {
		return -1
	}

	if a.Minor > b.Minor {
		return 1
	} else if a.Minor < b.Minor {
		return -1
	}

	return 0
}

func CheckKernelVersion(k, major, minor int) (bool, error) {
	var (
		v   *KernelVersion
		err error
	)
	if v, err = GetKernelVersion(); err != nil {
		return false, err
	}
	if CompareKernelVersion(*v, KernelVersion{Kernel: k, Major: major, Minor: minor}) < 0 {
		return false, nil
	}

	return true, nil
}
