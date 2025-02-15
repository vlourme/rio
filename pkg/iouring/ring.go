package iouring

import (
	"errors"
	"syscall"
	"unsafe"
)

const (
	SetupIOPoll uint32 = 1 << iota
	SetupSQPoll
	SetupSQAff
	SetupCQSize
	SetupClamp
	SetupAttachWQ
	SetupRDisabled
	SetupSubmitAll
	SetupCoopTaskRun
	SetupTaskRunFlag
	SetupSQE128
	SetupCQE32
	SetupSingleIssuer
	SetupDeferTaskRun
	SetupNoMmap
	SetupRegisteredFdOnly
)

const (
	FeatSingleMMap uint32 = 1 << iota
	FeatNoDrop
	FeatSubmitStable
	FeatRWCurPos
	FeatCurPersonality
	FeatFastPoll
	FeatPoll32Bits
	FeatSQPollNonfixed
	FeatExtArg
	FeatNativeWorkers
	FeatRcrcTags
	FeatCQESkip
	FeatLinkedFile
	FeatRegRegRing
)

const (
	MaxEntries     = 32768
	defaultEntries = MaxEntries / 2
)

type Options struct {
	Entries  uint32
	Flags    uint32
	Features uint32
	Buffer   []byte
	Params   *Params
}

type Option func(*Options) (err error)

func WithEntries(entries int) Option {
	return func(opts *Options) error {
		if entries > MaxEntries {
			return errors.New("entries too big")
		}
		if entries < 0 {
			entries = defaultEntries
		}
		opts.Entries = uint32(entries)
		return nil
	}
}

func WithFlags(flags uint32) Option {
	return func(opts *Options) error {
		opts.Flags = flags
		return nil
	}
}

func WithFeatures(features uint32) Option {
	return func(opts *Options) error {
		opts.Features = features
		return nil
	}
}

func WithMemoryBuffer(buffer []byte) Option {
	return func(opts *Options) error {
		if len(buffer) == 0 {
			return errors.New("buffer is empty")
		}
		opts.Buffer = buffer
		return nil
	}
}

func WithParams(params *Params) Option {
	return func(opts *Options) error {
		opts.Params = params
		return nil
	}
}

func New(options ...Option) (ring *Ring, err error) {
	opts := &Options{
		Entries: defaultEntries,
		Flags:   0,
		Params:  nil,
	}
	for _, opt := range options {
		err = opt(opts)
		if err != nil {
			return
		}
	}

	entries := opts.Entries

	var params *Params
	if params = opts.Params; params == nil {
		params = &Params{}
	}
	params.flags = opts.Flags
	params.features = opts.Features

	var buf unsafe.Pointer
	var bufSize uint64
	if mem := opts.Buffer; mem != nil {
		buf = unsafe.Pointer(unsafe.SliceData(mem))
		bufSize = uint64(len(mem))
		params.flags |= SetupNoMmap
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
	intFlags    uint8
	pad         [3]uint8
	pad2        uint32
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
		UnmapRings(sq, cq)
	} else if ring.intFlags&intFlagAppMem == 0 {
		_ = munmap(uintptr(unsafe.Pointer(sq.sqes)), uintptr(*sq.ringEntries)*unsafe.Sizeof(SubmissionQueueEntry{}))
		UnmapRings(sq, cq)
	}

	if ring.intFlags&intFlagRegRing != 0 {
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

func (ring *Ring) CloseFd() (uint, error) {
	if ring.features&FeatRegRegRing == 0 {
		return 0, syscall.EOPNOTSUPP
	}
	if (ring.intFlags & intFlagRegRing) == 0 {
		return 0, syscall.EINVAL
	}
	if ring.ringFd == -1 {
		return 0, syscall.EBADF
	}
	_ = syscall.Close(ring.ringFd)
	ring.ringFd = -1
	return 1, nil
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
