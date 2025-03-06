package aio

import (
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/kernel"
	"sync"
)

var (
	defaultRingFlags = uint32(0)
	defaultPlanOnce  = sync.Once{}
)

func DefaultIOURingSetupSchema() uint32 {
	defaultPlanOnce.Do(func() {
		version, versionErr := kernel.Get()
		if versionErr != nil {
			return
		}
		major, minor := version.Major, version.Minor
		// flags
		flags := uint32(0)
		if compareKernelVersion(major, minor, 5, 18) >= 0 {
			// submit all
			flags |= iouring.SetupSubmitAll
		}
		defaultRingFlags = flags
	})
	return defaultRingFlags
}

var (
	performanceRingFlags = uint32(0)
	performancePlanOnce  = sync.Once{}
)

func PerformanceIOURingSetupSchema() uint32 {
	performancePlanOnce.Do(func() {
		version, versionErr := kernel.Get()
		if versionErr != nil {
			return
		}
		major, minor := version.Major, version.Minor
		// flags
		flags := uint32(0)
		if compareKernelVersion(major, minor, 5, 11) >= 0 {
			// sq poll (it will make cpu busy)
			flags |= iouring.SetupSQPoll
			if compareKernelVersion(major, minor, 5, 18) >= 0 {
				// submit all
				flags |= iouring.SetupSubmitAll
				if compareKernelVersion(major, minor, 6, 0) >= 0 {
					// single issuer (dep on sq_poll)
					flags |= iouring.SetupSingleIssuer
				}
			}
		}
		performanceRingFlags = flags
	})
	return performanceRingFlags
}
