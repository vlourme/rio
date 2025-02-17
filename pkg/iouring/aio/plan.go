package aio

import (
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/kernel"
	"sync"
)

var (
	defaultRingFlags    = uint32(0)
	defaultRingFeatures = uint32(0)
	defaultPlanOnce     = sync.Once{}
)

func DefaultIOURingFlagsAndFeatures() (uint32, uint32) {
	defaultPlanOnce.Do(func() {
		version, versionErr := kernel.GetKernelVersion()
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
		// features
		features := uint32(0)
		if compareKernelVersion(major, minor, 5, 4) >= 0 {
			// single mmap
			features |= iouring.FeatSingleMMap
			if compareKernelVersion(major, minor, 5, 5) >= 0 {
				// submit stable
				features |= iouring.FeatSubmitStable
				if compareKernelVersion(major, minor, 5, 7) >= 0 {
					// fast poll
					features |= iouring.FeatFastPoll
					if compareKernelVersion(major, minor, 5, 11) >= 0 {
						// ext arg
						features |= iouring.FeatExtArg
						if compareKernelVersion(major, minor, 5, 12) >= 0 {
							// native workers
							features |= iouring.FeatNativeWorkers
						}
					}
				}
			}
		}

		defaultRingFlags = flags
		defaultRingFeatures = features
	})
	return defaultRingFlags, defaultRingFeatures
}

var (
	performRingFlags    = uint32(0)
	performRingFeatures = uint32(0)
	performPlanOnce     = sync.Once{}
)

func PerformIOURingFlagsAndFeatures() (uint32, uint32) {
	performPlanOnce.Do(func() {
		version, versionErr := kernel.GetKernelVersion()
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
		// features
		features := uint32(0)
		if compareKernelVersion(major, minor, 5, 4) >= 0 {
			// single mmap
			features |= iouring.FeatSingleMMap
			if compareKernelVersion(major, minor, 5, 5) >= 0 {
				// submit stable
				features |= iouring.FeatSubmitStable
				if compareKernelVersion(major, minor, 5, 7) >= 0 {
					// fast poll
					features |= iouring.FeatFastPoll
					if compareKernelVersion(major, minor, 5, 11) >= 0 {
						// ext arg
						features |= iouring.FeatExtArg
						// non fixed
						features |= iouring.FeatSQPollNonfixed
						if compareKernelVersion(major, minor, 5, 12) >= 0 {
							// native workers
							features |= iouring.FeatNativeWorkers
						}
					}
				}
			}
		}

		performRingFlags = flags
		performRingFeatures = features
	})
	return performRingFlags, performRingFeatures
}

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
