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

	if params.flags&IORING_SETUP_IOPOLL != 0 {
		flags |= IORING_SETUP_IOPOLL
	}
	if params.flags&IORING_SETUP_SQPOLL != 0 {
		if version.GTE(5, 13, 0) {
			flags |= IORING_SETUP_SQPOLL
		}
	}
	if flags&IORING_SETUP_SQPOLL != 0 && params.sqThreadIdle == 0 {
		params.sqThreadIdle = 15000
	}
	if params.flags&IORING_SETUP_SQ_AFF != 0 {
		if flags&IORING_SETUP_SQPOLL != 0 {
			flags |= IORING_SETUP_SQ_AFF
		}
	}
	if params.flags&IORING_SETUP_CQSIZE != 0 {
		if params.cqEntries > 0 {
			flags |= IORING_SETUP_CQSIZE
		}
	}
	if params.cqEntries > 0 && flags&IORING_SETUP_CQSIZE != 0 {
		params.sqEntries = 0
		params.cqEntries = 0
	}
	if params.flags&IORING_SETUP_CLAMP != 0 {
		flags |= IORING_SETUP_CLAMP
	}
	if params.flags&IORING_SETUP_ATTACH_WQ != 0 {
		if params.wqFd > 0 {
			flags |= IORING_SETUP_ATTACH_WQ
		}
	}
	if params.flags&IORING_SETUP_R_DISABLED != 0 {
		if version.GTE(5, 10, 0) {
			flags |= IORING_SETUP_R_DISABLED
		}
	}
	if params.flags&IORING_SETUP_SUBMIT_ALL != 0 {
		if version.GTE(5, 18, 0) {
			flags |= IORING_SETUP_SUBMIT_ALL
		}
	}
	if flags&IORING_SETUP_SQPOLL == 0 && params.flags&IORING_SETUP_COOP_TASKRUN != 0 {
		if version.GTE(5, 19, 0) {
			flags |= IORING_SETUP_COOP_TASKRUN
		}
	}

	if params.flags&IORING_SETUP_SINGLE_ISSUER != 0 {
		if version.GTE(6, 0, 0) {
			flags |= IORING_SETUP_SINGLE_ISSUER
		}
	}
	if flags&IORING_SETUP_SQPOLL == 0 && params.flags&IORING_SETUP_DEFER_TASKRUN != 0 {
		if version.GTE(6, 1, 0) && flags&IORING_SETUP_SINGLE_ISSUER != 0 {
			flags |= IORING_SETUP_DEFER_TASKRUN
		}
	}
	if flags&IORING_SETUP_SQPOLL == 0 && params.flags&IORING_SETUP_TASKRUN_FLAG != 0 {
		if version.GTE(5, 19, 0) && (flags&IORING_SETUP_COOP_TASKRUN != 0 || flags&IORING_SETUP_DEFER_TASKRUN != 0) {
			flags |= IORING_SETUP_TASKRUN_FLAG
		}
	}
	if params.flags&IORING_SETUP_SQE128 != 0 {
		if version.GTE(5, 19, 0) {
			flags |= IORING_SETUP_SQE128
		}
	}
	if params.flags&IORING_SETUP_CQE32 != 0 {
		if version.GTE(5, 19, 0) {
			flags |= IORING_SETUP_CQE32
		}
	}
	if params.flags&IORING_SETUP_NO_MMAP != 0 {
		if version.GTE(6, 5, 0) {
			flags |= IORING_SETUP_NO_MMAP
		}
	}
	if params.flags&IORING_SETUP_REGISTERED_FD_ONLY != 0 {
		if version.GTE(6, 5, 0) && flags&IORING_SETUP_NO_MMAP != 0 {
			flags |= IORING_SETUP_REGISTERED_FD_ONLY
		}
	}
	if params.flags&IORING_SETUP_NO_SQARRAY != 0 {
		if version.GTE(6, 6, 0) {
			flags |= IORING_SETUP_NO_SQARRAY
		}
	}
	if params.flags&IORING_SETUP_HYBRID_IOPOLL != 0 {
		if flags&IORING_SETUP_IOPOLL != 0 {
			flags |= IORING_SETUP_HYBRID_IOPOLL
		}
	}
	params.flags = flags
	return nil
}
