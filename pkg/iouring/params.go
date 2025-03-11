//go:build linux

package iouring

import "github.com/brickingsoft/rio/pkg/kernel"

type Params struct {
	sqEntries    uint32
	cqEntries    uint32
	flags        uint32
	sqThreadCPU  uint32
	sqThreadIdle uint32
	features     uint32
	wqFd         uint32
	resv         [3]uint32
	sqOff        SQRingOffsets
	cqOff        CQRingOffsets
}

func (params *Params) Validate() error {
	version, versionErr := kernel.Get()
	if versionErr != nil {
		return versionErr
	}
	major, minor := version.Major, version.Minor

	flags := uint32(0)

	if params.flags&SetupIOPoll != 0 {
		flags |= SetupIOPoll
	}
	if params.flags&SetupSQPoll != 0 {
		if kernel.CompareMajorAndMinor(major, minor, 5, 13) > -1 {
			flags |= SetupSQPoll
		}
	}
	if flags&SetupSQPoll != 0 && params.sqThreadIdle == 0 {
		params.sqThreadIdle = 15000
	}
	if params.flags&SetupSQAff != 0 {
		if flags&SetupSQPoll != 0 {
			flags |= SetupSQAff
		}
	}
	if params.flags&SetupCQSize != 0 {
		if params.cqEntries > 0 {
			flags |= SetupCQSize
		}
	}
	if params.cqEntries > 0 && flags&SetupCQSize != 0 {
		params.sqEntries = 0
		params.cqEntries = 0
	}
	if params.flags&SetupClamp != 0 {
		flags |= SetupClamp
	}
	if params.flags&SetupAttachWQ != 0 {
		if params.wqFd > 0 {
			flags |= SetupAttachWQ
		}
	}
	if params.flags&SetupRDisabled != 0 {
		if kernel.CompareMajorAndMinor(major, minor, 5, 10) > -1 {
			flags |= SetupRDisabled
		}
	}
	if params.flags&SetupSubmitAll != 0 {
		if kernel.CompareMajorAndMinor(major, minor, 5, 18) > -1 {
			flags |= SetupSubmitAll
		}
	}
	if params.flags&SetupCoopTaskRun != 0 {
		if kernel.CompareMajorAndMinor(major, minor, 5, 19) > -1 {
			flags |= SetupCoopTaskRun
		}
	}
	if params.flags&SetupTaskRunFlag != 0 {
		if kernel.CompareMajorAndMinor(major, minor, 5, 19) > -1 && flags&SetupCoopTaskRun != 0 {
			flags |= SetupTaskRunFlag
		}
	}
	if params.flags&SetupSQE128 != 0 {
		if kernel.CompareMajorAndMinor(major, minor, 5, 19) > -1 {
			flags |= SetupSQE128
		}
	}
	if params.flags&SetupCQE32 != 0 {
		if kernel.CompareMajorAndMinor(major, minor, 5, 19) > -1 {
			flags |= SetupCQE32
		}
	}
	if params.flags&SetupSingleIssuer != 0 {
		if kernel.CompareMajorAndMinor(major, minor, 6, 0) > -1 {
			flags |= SetupSingleIssuer
		}
	}
	if params.flags&SetupDeferTaskRun != 0 {
		if kernel.CompareMajorAndMinor(major, minor, 6, 1) > -1 && flags&SetupSingleIssuer != 0 {
			flags |= SetupDeferTaskRun
		}
	}
	if params.flags&SetupNoMmap != 0 {
		if kernel.CompareMajorAndMinor(major, minor, 6, 5) > -1 {
			flags |= SetupNoMmap
		}
	}
	if params.flags&SetupRegisteredFdOnly != 0 {
		if kernel.CompareMajorAndMinor(major, minor, 6, 5) > -1 && flags&SetupNoMmap != 0 {
			flags |= SetupRegisteredFdOnly
		}
	}
	if params.flags&SetupNoSQArray != 0 {
		if kernel.CompareMajorAndMinor(major, minor, 6, 6) > -1 {
			flags |= SetupNoSQArray
		}
	}
	if params.flags&SetupHybridIOPoll != 0 {
		if flags&SetupIOPoll != 0 {
			flags |= SetupHybridIOPoll
		}
	}
	params.flags = flags
	return nil
}
