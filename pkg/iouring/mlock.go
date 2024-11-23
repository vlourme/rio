package iouring

import (
	"os"
	"syscall"
)

// liburing: io_uring_mlock_size_params
func MlockSizeParams(entries uint32, p *Params) (uint64, error) {
	lp := &Params{}
	ring := NewRing()
	var cqEntries, sq uint32
	var pageSize uint64
	var err error

	err = ring.QueueInitParams(entries, lp)
	if err != nil {
		ring.QueueExit()
	}

	if lp.features&FeatNativeWorkers != 0 {
		return 0, nil
	}

	if entries == 0 {
		return 0, syscall.EINVAL
	}
	if entries > kernMaxEntries {
		if p.flags&SetupClamp == 0 {
			return 0, syscall.EINVAL
		}
		entries = kernMaxEntries
	}

	err = getSqCqEntries(entries, p, &sq, &cqEntries)
	if err != nil {
		return 0, err
	}

	pageSize = uint64(os.Getpagesize())

	return ringsSize(p, sq, cqEntries, pageSize), nil
}

// liburing: io_uring_mlock_size
func MlockSize(entries, flags uint32) (uint64, error) {
	p := &Params{}
	p.flags = flags

	return MlockSizeParams(entries, p)
}
