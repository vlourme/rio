//go:build linux

package kernel

import (
	"bytes"
	"fmt"
	"sync"

	"golang.org/x/sys/unix"
)

var (
	version     *Version = nil
	versionErr  error    = nil
	versionOnce          = sync.Once{}
)

const (
	firstNumberOfParts  = 2
	secondNumberOfParts = 1
)

func parseKernelVersion(kernelVersionStr string) (*Version, error) {
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

	return &Version{
		Kernel: kernel,
		Major:  major,
		Minor:  minor,
		Flavor: flavor,
	}, nil
}

func Get() (*Version, error) {
	versionOnce.Do(func() {
		uts := &unix.Utsname{}
		if err := unix.Uname(uts); err != nil {
			versionErr = err
			return
		}
		version, versionErr = parseKernelVersion(string(uts.Release[:bytes.IndexByte(uts.Release[:], 0)]))
	})
	return version, versionErr
}
