//go:build linux

package kernel

import (
	"bytes"
	"fmt"
	"sync"

	"golang.org/x/sys/unix"
)

var (
	version     = Version{}
	versionOnce = sync.Once{}
)

const (
	firstNumberOfParts  = 2
	secondNumberOfParts = 1
)

func parseKernelVersion(kernelVersionStr string) (major int, minor int, patch int, flavor string, err error) {
	var (
		parsed  int
		partial string
	)

	parsed, _ = fmt.Sscanf(kernelVersionStr, "%d.%d%s", &major, &minor, &partial)
	if parsed < firstNumberOfParts {
		err = fmt.Errorf("cannot parse kernel version: %s", kernelVersionStr)
		return
	}

	parsed, _ = fmt.Sscanf(partial, ".%d%s", &patch, &flavor)
	if parsed < secondNumberOfParts {
		flavor = partial
	}

	return
}

func Get() Version {
	versionOnce.Do(func() {
		uts := &unix.Utsname{}
		if err := unix.Uname(uts); err != nil {
			version.validate = false
			return
		}
		major, minor, patch, flavor, parseErr := parseKernelVersion(string(uts.Release[:bytes.IndexByte(uts.Release[:], 0)]))
		valid := parseErr == nil
		version.Major = major
		version.Minor = minor
		version.Patch = patch
		version.Flavor = flavor
		version.validate = valid
	})
	return version
}
