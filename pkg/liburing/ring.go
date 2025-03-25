//go:build linux

package liburing

import (
	"syscall"
	"unsafe"
)

func New(options ...Option) (ring *Ring, err error) {
	opts := Options{
		Entries:      DefaultEntries,
		Flags:        0,
		SQThreadCPU:  0,
		SQThreadIdle: 0,
		MemoryBuffer: nil,
	}
	for _, o := range options {
		if err = o(&opts); err != nil {
			return
		}
	}

	entries := opts.Entries

	params := &Params{}
	params.flags = opts.Flags
	params.sqThreadCPU = opts.SQThreadCPU
	params.sqThreadIdle = opts.SQThreadIdle
	params.wqFd = opts.WQFd

	var buf unsafe.Pointer
	var bufSize uint64
	if memoryBuffer := opts.MemoryBuffer; len(memoryBuffer) > 0 {
		buf = unsafe.Pointer(unsafe.SliceData(memoryBuffer))
		bufSize = uint64(len(memoryBuffer))
		params.flags |= SetupNoMmap
	}

	if err = params.Validate(); err != nil {
		return
	}

	ring = &Ring{
		sqRing: &SubmissionQueue{},
		cqRing: &CompletionQueue{},
	}
	err = ring.setup(entries, params, buf, bufSize)
	return
}

type Ring struct {
	sqRing      *SubmissionQueue
	cqRing      *CompletionQueue
	flags       uint32
	ringFd      int
	features    uint32
	enterRingFd int
	kind        uint8
	pad         [3]uint8
	pad2        uint32
}

func (ring *Ring) Flags() uint32 {
	return ring.flags
}

func (ring *Ring) Features() uint32 {
	return ring.features
}

func (ring *Ring) Close() (err error) {
	sq := ring.sqRing
	cq := ring.cqRing
	var sqeSize uintptr

	if sq.ringSize == 0 {
		sqeSize = unsafe.Sizeof(SubmissionQueueEntry{})
		if ring.flags&SetupSQE128 != 0 {
			sqeSize += 64
		}
		_ = munmap(uintptr(unsafe.Pointer(sq.sqes)), sqeSize*uintptr(*sq.ringEntries))
		unmapRings(sq, cq)
	} else if ring.kind&appMemRing == 0 {
		_ = munmap(uintptr(unsafe.Pointer(sq.sqes)), uintptr(*sq.ringEntries)*unsafe.Sizeof(SubmissionQueueEntry{}))
		unmapRings(sq, cq)
	}

	if ring.kind&regRing != 0 {
		_, _ = ring.UnregisterRingFd()
	}
	if ring.ringFd != -1 {
		err = syscall.Close(ring.ringFd)
	}
	return
}

func (ring *Ring) Fd() int {
	return ring.ringFd
}

func (ring *Ring) EnableRings() (uint, error) {
	return ring.doRegister(RegisterEnableRings, unsafe.Pointer(nil), 0)
}

func (ring *Ring) CloseFd() error {
	if ring.features&FeatRegRegRing == 0 {
		return syscall.EOPNOTSUPP
	}
	if ring.kind&regRing == 0 {
		return syscall.EINVAL
	}
	if ring.ringFd == -1 {
		return syscall.EBADF
	}
	_ = syscall.Close(ring.ringFd)
	ring.ringFd = -1
	return nil
}

func (ring *Ring) Probe() (*Probe, error) {
	probe := &Probe{}
	_, err := ring.RegisterProbe(probe, probeOpsSize)
	if err != nil {
		return nil, err
	}
	return probe, nil
}

func (ring *Ring) DontFork() error {
	var length uintptr
	var err error

	if ring.sqRing.ringPtr == nil || ring.sqRing.sqes == nil || ring.cqRing.ringPtr == nil {
		return syscall.EINVAL
	}

	length = unsafe.Sizeof(SubmissionQueueEntry{})
	if ring.flags&SetupSQE128 != 0 {
		length += 64
	}
	length *= uintptr(*ring.sqRing.ringEntries)
	err = madvise(uintptr(unsafe.Pointer(ring.sqRing.sqes)), length, syscall.MADV_DONTFORK)
	if err != nil {
		return err
	}

	length = uintptr(ring.sqRing.ringSize)
	err = madvise(uintptr(ring.sqRing.ringPtr), length, syscall.MADV_DONTFORK)
	if err != nil {
		return err
	}

	if ring.cqRing.ringPtr != ring.sqRing.ringPtr {
		length = uintptr(ring.cqRing.ringSize)
		err = madvise(uintptr(ring.cqRing.ringPtr), length, syscall.MADV_DONTFORK)
		if err != nil {
			return err
		}
	}

	return nil
}
