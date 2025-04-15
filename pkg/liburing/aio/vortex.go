//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"os"
	"time"
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
		Entries:           0,
		Flags:             liburing.IORING_SETUP_COOP_TASKRUN | liburing.IORING_SETUP_SINGLE_ISSUER,
		SQThreadIdle:      0,
		SendZCEnabled:     false,
		MultishotDisabled: false,
		BufferAndRingConfig: BufferAndRingConfig{
			Size:        os.Getpagesize(),
			Count:       16,
			IdleTimeout: 5 * time.Second,
		},
		WaitCQEIdleTimeout: 15 * time.Second,
		WaitCQETimeCurve: Curve{
			//{16, 1 * time.Microsecond},
			//{32, 5 * time.Microsecond},
			//{64, 10 * time.Microsecond},
			{16, 10 * time.Microsecond},
			{32, 20 * time.Microsecond},
			{64, 40 * time.Microsecond},
			//{16, 200 * time.Microsecond},
			//{32, 300 * time.Microsecond},
			//{64, 500 * time.Microsecond},
		},
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
