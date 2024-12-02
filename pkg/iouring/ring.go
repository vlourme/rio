//go:build linux

package iouring

import (
	"golang.org/x/sys/unix"
	"math"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	liburingUdataTimeout    uint64 = math.MaxUint64
	nSig                           = 65
	szDivider                      = 8
	registerRingFdOffset           = uint32(4294967295)
	regIOWQMaxWorkersNrArgs        = 2
)

const (
	IntFlagRegRing    uint8 = 1
	IntFlagRegRegRing uint8 = 2
	IntFlagAppMem     uint8 = 4
)

func NewRing() *Ring {
	return &Ring{
		sqRing: &SubmissionQueue{},
		cqRing: &CompletionQueue{},
	}
}

func CreateRing(entries uint32) (*Ring, error) {
	var (
		ring  = NewRing()
		flags uint32
	)

	err := ring.QueueInit(entries, flags)
	if err != nil {
		return nil, err
	}

	return ring, nil
}

type Ring struct {
	sqRing      *SubmissionQueue
	cqRing      *CompletionQueue
	flags       uint32
	ringFd      int
	features    uint32
	enterRingFd int
	intFlags    uint8
	// nolint: unused
	pad [3]uint8
	// nolint: unused
	pad2 uint32
}

// liburing: io_uring_cqe_shift
func (ring *Ring) cqeShift() uint32 {
	if ring.flags&SetupCQE32 != 0 {
		return 1
	}

	return 0
}

// liburing: io_uring_cqe_index
func (ring *Ring) cqeIndex(ptr, mask uint32) uintptr {
	return uintptr((ptr & mask) << ring.cqeShift())
}

// liburing: io_uring_for_each_cqe - https://manpages.debian.org/unstable/liburing-dev/io_uring_for_each_cqe.3.en.html
func (ring *Ring) ForEachCQE(callback func(cqe *CompletionQueueEvent)) {
	var cqe *CompletionQueueEvent
	for head := atomic.LoadUint32(ring.cqRing.head); ; head++ {
		if head != atomic.LoadUint32(ring.cqRing.tail) {
			cqeIndex := ring.cqeIndex(head, *ring.cqRing.ringMask)
			cqe = (*CompletionQueueEvent)(
				unsafe.Add(unsafe.Pointer(ring.cqRing.cqes), cqeIndex*unsafe.Sizeof(CompletionQueueEvent{})),
			)
			callback(cqe)
		} else {
			break
		}
	}
}

// liburing: io_uring_cq_advance - https://manpages.debian.org/unstable/liburing-dev/io_uring_cq_advance.3.en.html
func (ring *Ring) CQAdvance(numberOfCQEs uint32) {
	atomic.StoreUint32(ring.cqRing.head, *ring.cqRing.head+numberOfCQEs)
}

// liburing: io_uring_cqe_seen - https://manpages.debian.org/unstable/liburing-dev/io_uring_cqe_seen.3.en.html
func (ring *Ring) CQESeen(event *CompletionQueueEvent) {
	if event != nil {
		ring.CQAdvance(1)
	}
}

// liburing: io_uring_sq_ready - https://manpages.debian.org/unstable/liburing-dev/io_uring_sq_ready.3.en.html
func (ring *Ring) SQReady() uint32 {
	khead := *ring.sqRing.head

	if ring.flags&SetupSQPoll != 0 {
		khead = atomic.LoadUint32(ring.sqRing.head)
	}

	return ring.sqRing.sqeTail - khead
}

// liburing: io_uring_sq_space_left - https://manpages.debian.org/unstable/liburing-dev/io_uring_sq_space_left.3.en.html
func (ring *Ring) SQSpaceLeft() uint32 {
	return *ring.sqRing.ringEntries - ring.SQReady()
}

// liburing: io_uring_sqring_wait - https://manpages.debian.org/unstable/liburing-dev/io_uring_sqring_wait.3.en.html
func (ring *Ring) SQRingWait() (uint, error) {
	if ring.flags&SetupSQPoll == 0 {
		return 0, nil
	}
	if ring.SQSpaceLeft() != 0 {
		return 0, nil
	}

	return ring.internalSQRingWait()
}

// liburing: io_uring_cq_ready - https://manpages.debian.org/unstable/liburing-dev/io_uring_cq_ready.3.en.html
func (ring *Ring) CQReady() uint32 {
	return atomic.LoadUint32(ring.cqRing.tail) - *ring.cqRing.head
}

// liburing: io_uring_cq_has_overflow - https://manpages.debian.org/unstable/liburing-dev/io_uring_cq_has_overflow.3.en.html
func (ring *Ring) CQHasOverflow() bool {
	return atomic.LoadUint32(ring.sqRing.flags)&SQCQOverflow != 0
}

// liburing: io_uring_cq_eventfd_enabled
func (ring *Ring) CQEventfdEnabled() bool {
	if *ring.cqRing.flags == 0 {
		return true
	}

	return !(*ring.cqRing.flags&CQEventFdDisabled != 0)
}

// liburing: io_uring_cq_eventfd_toggle
func (ring *Ring) CqEventfdToggle(enabled bool) error {
	var flags uint32

	if enabled == ring.CQEventfdEnabled() {
		return nil
	}

	if *ring.cqRing.flags == 0 {
		return syscall.EOPNOTSUPP
	}

	flags = *ring.cqRing.flags

	if enabled {
		flags &= ^CQEventFdDisabled
	} else {
		flags |= CQEventFdDisabled
	}

	atomic.StoreUint32(ring.cqRing.flags, flags)

	return nil
}

// liburing: io_uring_wait_cqe_nr - https://manpages.debian.org/unstable/liburing-dev/io_uring_wait_cqe_nr.3.en.html
func (ring *Ring) WaitCQENr(waitNr uint32) (*CompletionQueueEvent, error) {
	return ring.internalGetCQE(0, waitNr, nil)
}

// liburing: io_uring_peek_cqe - https://manpages.debian.org/unstable/liburing-dev/io_uring_peek_cqe.3.en.html
func (ring *Ring) PeekCQE() (*CompletionQueueEvent, error) {
	cqe, err := internalPeekCQE(ring, nil)
	if err == nil && cqe != nil {
		return cqe, nil
	}

	return ring.WaitCQENr(0)
}

// liburing: io_uring_wait_cqe - https://manpages.debian.org/unstable/liburing-dev/io_uring_wait_cqe.3.en.html
func (ring *Ring) WaitCQE() (*CompletionQueueEvent, error) {
	// return ring.WaitCQENr(1)
	cqe, err := internalPeekCQE(ring, nil)
	if err == nil && cqe != nil {
		return cqe, nil
	}

	return ring.WaitCQENr(1)
}

// liburing: _io_uring_get_sqe
func privateGetSQE(ring *Ring) *SubmissionQueueEntry {
	sq := ring.sqRing
	var head, next uint32
	var shift int

	if ring.flags&SetupSQE128 != 0 {
		shift = 1
	}
	head = atomic.LoadUint32(sq.head)
	next = sq.sqeTail + 1
	if next-head <= *sq.ringEntries {
		sqe := (*SubmissionQueueEntry)(
			unsafe.Add(unsafe.Pointer(ring.sqRing.sqes),
				uintptr((sq.sqeTail&*sq.ringMask)<<shift)*unsafe.Sizeof(SubmissionQueueEntry{})),
		)
		sq.sqeTail = next

		return sqe
	}

	return nil
}

// liburing: io_uring_get_sqe - https://manpages.debian.org/unstable/liburing-dev/io_uring_get_sqe.3.en.html
func (ring *Ring) GetSQE() *SubmissionQueueEntry {
	return privateGetSQE(ring)
}

// liburing: get_data
type getData struct {
	submit   uint32
	waitNr   uint32
	getFlags uint32
	sz       int
	hasTS    bool
	arg      unsafe.Pointer
}

// liburing: __io_uring_get_cqe
func (ring *Ring) internalGetCQE(submit uint32, waitNr uint32, sigmask *unix.Sigset_t) (*CompletionQueueEvent, error) {
	data := getData{
		submit:   submit,
		waitNr:   waitNr,
		getFlags: 0,
		sz:       nSig / szDivider,
		arg:      unsafe.Pointer(sigmask),
	}

	cqe, err := ring.privateGetCQE(&data)
	runtime.KeepAlive(data)

	return cqe, err
}

// liburing: sq_ring_needs_enter
func (ring *Ring) sqRingNeedsEnter(submit uint32, flags *uint32) bool {
	if submit == 0 {
		return false
	}

	if (ring.flags & SetupSQPoll) == 0 {
		return true
	}

	if atomic.LoadUint32(ring.sqRing.flags)&SQNeedWakeup != 0 {
		*flags |= EnterSQWakeup

		return true
	}

	return false
}

// liburing: cq_ring_needs_flush
func (ring *Ring) cqRingNeedsFlush() bool {
	return atomic.LoadUint32(ring.sqRing.flags)&(SQCQOverflow|SQTaskrun) != 0
}

// liburing: cq_ring_needs_enter
func (ring *Ring) cqRingNeedsEnter() bool {
	return (ring.flags&SetupIOPoll) != 0 || ring.cqRingNeedsFlush()
}

// liburing: io_uring_get_events - https://manpages.debian.org/unstable/liburing-dev/io_uring_get_events.3.en.html
func (ring *Ring) GetEvents() (uint, error) {
	flags := EnterGetEvents

	if ring.intFlags&IntFlagRegRing != 0 {
		flags |= EnterRegisteredRing
	}

	return ring.Enter(0, 0, flags, nil)
}

// liburing: io_uring_peek_batch_cqe
func (ring *Ring) PeekBatchCQE(cqes []*CompletionQueueEvent) uint32 {
	var ready uint32
	var overflowChecked bool
	var shift int

	if ring.flags&SetupCQE32 != 0 {
		shift = 1
	}

	count := uint32(len(cqes))

again:
	ready = ring.CQReady()
	if ready != 0 {
		head := *ring.cqRing.head
		mask := *ring.cqRing.ringMask
		last := head + count
		if count > ready {
			count = ready
		}
		for i := 0; head != last; head, i = head+1, i+1 {
			cqes[i] = (*CompletionQueueEvent)(
				unsafe.Add(
					unsafe.Pointer(ring.cqRing.cqes),
					uintptr((head&mask)<<shift)*unsafe.Sizeof(CompletionQueueEvent{}),
				),
			)
		}

		return count
	}

	if overflowChecked {
		return 0
	}

	if ring.cqRingNeedsFlush() {
		_, _ = ring.GetEvents()
		overflowChecked = true

		goto again
	}

	return 0
}

// liburing: __io_uring_flush_sq
func (ring *Ring) internalFlushSQ() uint32 {
	sq := ring.sqRing
	tail := sq.sqeTail

	if sq.sqeHead != tail {
		sq.sqeHead = tail
		atomic.StoreUint32(sq.tail, tail)
	}

	return tail - atomic.LoadUint32(sq.head)
}

// liburing: io_uring_wait_cqes_new
func (ring *Ring) WaitCQEsNew(
	waitNr uint32, ts *syscall.Timespec, sigmask *unix.Sigset_t,
) (*CompletionQueueEvent, error) {
	var arg *GetEventsArg
	var data *getData

	arg = &GetEventsArg{
		sigMask:   uint64(uintptr(unsafe.Pointer(sigmask))),
		sigMaskSz: nSig / szDivider,
		ts:        uint64(uintptr(unsafe.Pointer(ts))),
	}

	data = &getData{
		waitNr:   waitNr,
		getFlags: EnterExtArg,
		sz:       int(unsafe.Sizeof(GetEventsArg{})),
		hasTS:    true,
		arg:      unsafe.Pointer(arg),
	}

	cqe, err := ring.privateGetCQE(data)
	runtime.KeepAlive(data)

	return cqe, err
}

// liburing: __io_uring_submit_timeout
func (ring *Ring) internalSubmitTimeout(waitNr uint32, ts *syscall.Timespec) (uint32, error) {
	var sqe *SubmissionQueueEntry
	var err error

	/*
	 * If the SQ ring is full, we may need to submit IO first
	 */
	sqe = ring.GetSQE()
	if sqe == nil {
		_, err = ring.Submit()
		if err != nil {
			return 0, err
		}
		sqe = ring.GetSQE()
		if sqe == nil {
			return 0, syscall.EAGAIN
		}
	}
	sqe.PrepareTimeout(ts, waitNr, 0)
	sqe.UserData = liburingUdataTimeout

	return ring.internalFlushSQ(), nil
}

// liburing: io_uring_wait_cqes - https://manpages.debian.org/unstable/liburing-dev/io_uring_wait_cqes.3.en.html
func (ring *Ring) WaitCQEs(waitNr uint32, ts *syscall.Timespec, sigmask *unix.Sigset_t) (*CompletionQueueEvent, error) {
	var toSubmit uint32
	var err error

	if ts != nil {
		if ring.features&FeatExtArg != 0 {
			return ring.WaitCQEsNew(waitNr, ts, sigmask)
		}
		toSubmit, err = ring.internalSubmitTimeout(waitNr, ts)
		if err != nil {
			return nil, err
		}
	}

	return ring.internalGetCQE(toSubmit, waitNr, sigmask)
}

// liburing: io_uring_submit_and_wait_timeout - https://manpages.debian.org/unstable/liburing-dev/io_uring_submit_and_wait_timeout.3.en.html
func (ring *Ring) SubmitAndWaitTimeout(
	waitNr uint32, ts *syscall.Timespec, sigmask *unix.Sigset_t,
) (*CompletionQueueEvent, error) {
	var toSubmit uint32
	var err error
	var cqe *CompletionQueueEvent

	if ts != nil {
		if ring.features&FeatExtArg != 0 {
			arg := GetEventsArg{
				sigMask:   uint64(uintptr(unsafe.Pointer(sigmask))),
				sigMaskSz: nSig / szDivider,
				ts:        uint64(uintptr(unsafe.Pointer(ts))),
			}
			data := getData{
				submit:   ring.internalFlushSQ(),
				waitNr:   waitNr,
				getFlags: EnterExtArg,
				sz:       int(unsafe.Sizeof(arg)),
				hasTS:    ts != nil,
				arg:      unsafe.Pointer(&arg),
			}

			cqe, err = ring.privateGetCQE(&data)
			runtime.KeepAlive(data)

			return cqe, err
		}
		toSubmit, err = ring.internalSubmitTimeout(waitNr, ts)
		if err != nil {
			return cqe, err
		}
	} else {
		toSubmit = ring.internalFlushSQ()
	}

	return ring.internalGetCQE(toSubmit, waitNr, sigmask)
}

// liburing: _io_uring_get_cqe
func (ring *Ring) privateGetCQE(data *getData) (*CompletionQueueEvent, error) {
	var cqe *CompletionQueueEvent
	var looped bool
	var err error

	for {
		var needEnter bool
		var flags uint32
		var nrAvailable uint32
		var ret uint
		var localErr error

		cqe, localErr = internalPeekCQE(ring, &nrAvailable)
		if localErr != nil {
			if err == nil {
				err = localErr
			}

			break
		}
		if cqe == nil && data.waitNr == 0 && data.submit == 0 {
			if looped || !ring.cqRingNeedsEnter() {
				if err == nil {
					err = unix.EAGAIN
				}

				break
			}
			needEnter = true
		}
		if data.waitNr > nrAvailable || needEnter {
			flags = EnterGetEvents | data.getFlags
			needEnter = true
		}
		if ring.sqRingNeedsEnter(data.submit, &flags) {
			needEnter = true
		}
		if !needEnter {
			break
		}
		if looped && data.hasTS {
			arg := (*GetEventsArg)(data.arg)
			if cqe == nil && arg.ts != 0 && err == nil {
				err = unix.ETIME
			}

			break
		}
		if ring.intFlags&IntFlagRegRing != 0 {
			flags |= EnterRegisteredRing
		}
		ret, localErr = ring.Enter2(data.submit, data.waitNr, flags, data.arg, data.sz)
		if localErr != nil {
			if err == nil {
				err = localErr
			}

			break
		}
		data.submit -= uint32(ret)
		if cqe != nil {
			break
		}
		if !looped {
			looped = true
			err = localErr
		}
	}

	return cqe, err
}

// liburing: io_uring_wait_cqe_timeout - https://manpages.debian.org/unstable/liburing-dev/io_uring_wait_cqe_timeout.3.en.html
func (ring *Ring) WaitCQETimeout(ts *syscall.Timespec) (*CompletionQueueEvent, error) {
	return ring.WaitCQEs(1, ts, nil)
}

// liburing: __io_uring_submit
func (ring *Ring) internalSubmit(submitted uint32, waitNr uint32, getEvents bool) (uint, error) {
	cqNeedsEnter := getEvents || waitNr != 0 || ring.cqRingNeedsEnter()

	var flags uint32
	var ret uint
	var err error

	flags = 0
	if ring.sqRingNeedsEnter(submitted, &flags) || cqNeedsEnter {
		if cqNeedsEnter {
			flags |= EnterGetEvents
		}
		if ring.intFlags&IntFlagRegRing != 0 {
			flags |= EnterRegisteredRing
		}

		ret, err = ring.Enter(submitted, waitNr, flags, nil)
		if err != nil {
			return 0, err
		}
	} else {
		ret = uint(submitted)
	}

	return ret, nil
}

// liburing: __io_uring_submit_and_wait
func (ring *Ring) internalSubmitAndWait(waitNr uint32) (uint, error) {
	return ring.internalSubmit(ring.internalFlushSQ(), waitNr, false)
}

// liburing: io_uring_submit - https://manpages.debian.org/unstable/liburing-dev/io_uring_submit.3.en.html
func (ring *Ring) Submit() (uint, error) {
	return ring.internalSubmitAndWait(0)
}

// liburing: io_uring_submit_and_wait - https://manpages.debian.org/unstable/liburing-dev/io_uring_submit_and_wait.3.en.html
func (ring *Ring) SubmitAndWait(waitNr uint32) (uint, error) {
	return ring.internalSubmitAndWait(waitNr)
}

// liburing: io_uring_submit_and_get_events - https://manpages.debian.org/unstable/liburing-dev/io_uring_submit_and_get_events.3.en.html
func (ring *Ring) SubmitAndGetEvents() (uint, error) {
	return ring.internalSubmit(ring.internalFlushSQ(), 0, true)
}

// __io_uring_sqring_wait
func (ring *Ring) internalSQRingWait() (uint, error) {
	flags := EnterSQWait

	if ring.intFlags&IntFlagRegRegRing != 0 {
		flags |= EnterRegisteredRing
	}

	return ring.Enter(0, 0, flags, nil)
}

// liburing: io_uring_queue_mmap
func (ring *Ring) QueueMmap(fd int, p *Params) error {
	return Mmap(fd, p, ring.sqRing, ring.cqRing)
}

// liburing: io_uring_ring_dontfork
func (ring *Ring) RingDontFork() error {
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
	err = sysMadvise(uintptr(unsafe.Pointer(ring.sqRing.sqes)), length, syscall.MADV_DONTFORK)
	if err != nil {
		return err
	}

	length = uintptr(ring.sqRing.ringSize)
	err = sysMadvise(uintptr(ring.sqRing.ringPtr), length, syscall.MADV_DONTFORK)
	if err != nil {
		return err
	}

	if ring.cqRing.ringPtr != ring.sqRing.ringPtr {
		length = uintptr(ring.cqRing.ringSize)
		err = sysMadvise(uintptr(ring.cqRing.ringPtr), length, syscall.MADV_DONTFORK)
		if err != nil {
			return err
		}
	}

	return nil
}

// liburing: __io_uring_queue_init_params
func (ring *Ring) internalQueueInitParams(entries uint32, p *Params, buf unsafe.Pointer, bufSize uint64) error {
	var fd int
	var sqEntries, index uint32
	var err error

	if p.flags&SetupRegisteredFdOnly != 0 && p.flags&SetupNoMmap == 0 {
		return syscall.EINVAL
	}

	if p.flags&SetupNoMmap != 0 {
		_, err = allocHuge(entries, p, ring.sqRing, ring.cqRing, buf, bufSize)
		if err != nil {
			return err
		}
		if buf != nil {
			ring.intFlags |= IntFlagAppMem
		}
	}

	//fdPtr, err := Setup(entries, p)

	fdPtr, _, errno := syscall.Syscall(sysSetup, uintptr(entries), uintptr(unsafe.Pointer(p)), 0)
	if errno != 0 {
		if p.flags&SetupNoMmap != 0 && ring.intFlags&IntFlagAppMem == 0 {
			_ = sysMunmap(uintptr(unsafe.Pointer(ring.sqRing.sqes)), 1)
			UnmapRings(ring.sqRing, ring.cqRing)
		}

		return errno
	}
	fd = int(fdPtr)

	if p.flags&SetupNoMmap == 0 {
		err = ring.QueueMmap(fd, p)
		if err != nil {
			syscall.Close(fd)

			return err
		}
	} else {
		SetupRingPointers(p, ring.sqRing, ring.cqRing)
	}

	sqEntries = *ring.sqRing.ringEntries
	for index = 0; index < sqEntries; index++ {
		*(*uint32)(
			unsafe.Add(unsafe.Pointer(ring.sqRing.array),
				index*uint32(unsafe.Sizeof(uint32(0))))) = index
	}

	ring.features = p.features
	ring.flags = p.flags
	ring.enterRingFd = fd
	if p.flags&SetupRegisteredFdOnly != 0 {
		ring.ringFd = -1
		ring.intFlags |= IntFlagRegRing | IntFlagRegRegRing
	} else {
		ring.ringFd = fd
	}

	return nil
}

// liburing: io_uring_queue_init_mem
func (ring *Ring) QueueInitMem(entries uint32, p *Params, buf unsafe.Pointer, bufSize uint64) error {
	// should already be set...
	p.flags |= SetupNoMmap

	return ring.internalQueueInitParams(entries, p, buf, bufSize)
}

// liburing: io_uring_queue_init_params - https://manpages.debian.org/unstable/liburing-dev/io_uring_queue_init_params.3.en.html
func (ring *Ring) QueueInitParams(entries uint32, p *Params) error {
	return ring.internalQueueInitParams(entries, p, nil, 0)
}

// liburing: io_uring_queue_init - https://manpages.debian.org/unstable/liburing-dev/io_uring_queue_init.3.en.html
func (ring *Ring) QueueInit(entries uint32, flags uint32) error {
	params := &Params{
		flags: flags,
	}

	return ring.QueueInitParams(entries, params)
}

// liburing: io_uring_queue_exit - https://manpages.debian.org/unstable/liburing-dev/io_uring_queue_exit.3.en.html
func (ring *Ring) QueueExit() {
	sq := ring.sqRing
	cq := ring.cqRing
	var sqeSize uintptr

	if sq.ringSize == 0 {
		sqeSize = unsafe.Sizeof(SubmissionQueueEntry{})
		if ring.flags&SetupSQE128 != 0 {
			sqeSize += 64
		}
		_ = sysMunmap(uintptr(unsafe.Pointer(sq.sqes)), sqeSize*uintptr(*sq.ringEntries))
		UnmapRings(sq, cq)
	} else if ring.intFlags&IntFlagAppMem == 0 {
		_ = sysMunmap(uintptr(unsafe.Pointer(sq.sqes)), uintptr(*sq.ringEntries)*unsafe.Sizeof(SubmissionQueueEntry{}))
		UnmapRings(sq, cq)
	}

	if ring.intFlags&IntFlagRegRing != 0 {
		_, _ = ring.UnregisterRingFd()
	}
	if ring.ringFd != -1 {
		syscall.Close(ring.ringFd)
	}
}

// liburing: br_setup
func (ring *Ring) brSetup(nentries uint32, bgid uint16, flags uint32) (*BufAndRing, error) {
	var br *BufAndRing
	var reg BufReg
	var ringSize, brPtr uintptr
	var err error

	reg = BufReg{}
	ringSize = uintptr(nentries) * unsafe.Sizeof(BufAndRing{})
	brPtr, err = mmap(
		0, ringSize, syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANONYMOUS|syscall.MAP_PRIVATE, -1, 0)
	if err != nil {
		return nil, err
	}
	br = (*BufAndRing)(unsafe.Pointer(brPtr))

	reg.RingAddr = uint64(uintptr(unsafe.Pointer(br)))
	reg.RingEntries = nentries
	reg.Bgid = bgid

	_, err = ring.RegisterBufferRing(&reg, flags)
	if err != nil {
		_ = sysMunmap(uintptr(unsafe.Pointer(br)), ringSize)

		return nil, err
	}

	return br, nil
}

// liburing: io_uring_setup_buf_ring - https://manpages.debian.org/unstable/liburing-dev/io_uring_setup_buf_ring.3.en.html
func (ring *Ring) SetupBufRing(nentries uint32, bgid int, flags uint32) (*BufAndRing, error) {
	br, err := ring.brSetup(nentries, uint16(bgid), flags)
	if br != nil {
		br.BufRingInit()
	}

	return br, err
}

// liburing: io_uring_free_buf_ring - https://manpages.debian.org/unstable/liburing-dev/io_uring_free_buf_ring.3.en.html
func (ring *Ring) FreeBufRing(bgid int) error {
	_, err := ring.UnregisterBufferRing(bgid)

	return err
}

func (ring *Ring) RingFd() int {
	return ring.ringFd
}
