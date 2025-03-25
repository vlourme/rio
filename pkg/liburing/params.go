//go:build linux

package liburing

import (
	"errors"
)

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
	version := GetVersion()
	if version.Invalidate() {
		return errors.New("get kernel version failed")
	}

	flags := uint32(0)

	if params.flags&SetupIOPoll != 0 {
		flags |= SetupIOPoll
	}
	if params.flags&SetupSQPoll != 0 {
		if version.GTE(5, 13, 0) {
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
		if version.GTE(5, 10, 0) {
			flags |= SetupRDisabled
		}
	}
	if params.flags&SetupSubmitAll != 0 {
		if version.GTE(5, 18, 0) {
			flags |= SetupSubmitAll
		}
	}
	if params.flags&SetupCoopTaskRun != 0 {
		if version.GTE(5, 19, 0) {
			flags |= SetupCoopTaskRun
		}
	}
	if params.flags&SetupTaskRunFlag != 0 {
		if version.GTE(5, 19, 0) && flags&SetupCoopTaskRun != 0 {
			flags |= SetupTaskRunFlag
		}
	}
	if params.flags&SetupSQE128 != 0 {
		if version.GTE(5, 19, 0) {
			flags |= SetupSQE128
		}
	}
	if params.flags&SetupCQE32 != 0 {
		if version.GTE(5, 19, 0) {
			flags |= SetupCQE32
		}
	}
	if params.flags&SetupSingleIssuer != 0 {
		if version.GTE(6, 0, 0) {
			flags |= SetupSingleIssuer
		}
	}
	if params.flags&SetupDeferTaskRun != 0 {
		if version.GTE(6, 1, 0) && flags&SetupSingleIssuer != 0 {
			flags |= SetupDeferTaskRun
		}
	}
	if params.flags&SetupNoMmap != 0 {
		if version.GTE(6, 5, 0) {
			flags |= SetupNoMmap
		}
	}
	if params.flags&SetupRegisteredFdOnly != 0 {
		if version.GTE(6, 5, 0) && flags&SetupNoMmap != 0 {
			flags |= SetupRegisteredFdOnly
		}
	}
	if params.flags&SetupNoSQArray != 0 {
		if version.GTE(6, 6, 0) {
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
