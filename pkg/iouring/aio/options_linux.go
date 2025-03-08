//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/kernel"
)

func defaultIOURingSetupFlags() uint32 {
	version, versionErr := kernel.Get()
	if versionErr != nil {
		return 0
	}
	major, minor := version.Major, version.Minor
	// flags
	flags := uint32(0)
	if compareKernelVersion(major, minor, 5, 18) >= 0 {
		// submit all
		flags |= iouring.SetupSubmitAll
	}
	return flags
}

func performanceIOURingSetupFlags() uint32 {
	version, versionErr := kernel.Get()
	if versionErr != nil {
		return 0
	}
	major, minor := version.Major, version.Minor
	// flags
	flags := uint32(0)
	if compareKernelVersion(major, minor, 5, 13) >= 0 {
		// submit all
		flags |= iouring.SetupSQPoll
		if compareKernelVersion(major, minor, 5, 18) >= 0 {
			// submit all
			flags |= iouring.SetupSubmitAll
			if compareKernelVersion(major, minor, 6, 0) >= 0 {
				// submit all
				flags |= iouring.SetupSingleIssuer
			}
		}
	}
	return flags
}
