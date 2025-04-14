//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
)

func Open(options ...Option) (v AsyncIO, err error) {
	// version check
	if !liburing.VersionEnable(6, 8, 0) {
		// support
		// * io_uring_setup_buf_ring 5.19
		// * io_uring_register_ring_fd 5.18
		// * io_uring_prep_msg_ring  6.0
		// * io_uring_prep_recv_multishot  6.0
		// * io_uring_prep_cmd 6.7
		// * io_uring_prep_fixed_fd_install 6.8
		err = errors.New("kernel version must >= 6.8")
		return
	}

	// options
	opt := Options{
		Entries: 0,
		Flags:   liburing.IORING_SETUP_COOP_TASKRUN | liburing.IORING_SETUP_SINGLE_ISSUER,
		//Flags:               liburing.IORING_SETUP_SQPOLL | liburing.IORING_SETUP_SQ_AFF,
		SQThreadIdle:        0,
		SendZCEnabled:       false,
		MultishotDisabled:   false,
		BufferAndRingConfig: BufferAndRingConfig{},
	}
	for _, option := range options {
		option(&opt)
	}
	// event loop group
	group, groupErr := newEventLoopGroup(opt)
	if groupErr != nil {
		err = groupErr
		return
	}

	// vortex
	v = &Vortex{
		group:             group,
		sendZCEnabled:     opt.SendZCEnabled,
		multishotDisabled: opt.MultishotDisabled,
	}
	return
}

type Vortex struct {
	group             *EventLoopGroup
	sendZCEnabled     bool
	multishotDisabled bool
}

func (vortex *Vortex) Close() (err error) {
	err = vortex.group.Close()
	return
}
