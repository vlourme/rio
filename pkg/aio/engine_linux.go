//go:build linux

package aio

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"
)

func (engine *Engine) Start() {
	// todo cylinders -> one cylinder one ring/iocp/kqueue loop
	// todo op with ring -> load balance to load cylinder
	settings := ResolveSettings[IOURingSettings](engine.settings)
	// entries
	entries := settings.Entries
	if entries == 0 {
		cpuNum := runtime.NumCPU() * 2
		entries = uint32(cpuNum * 1024)
	}
	// param
	param := &settings.Param
	// batch
	batch := settings.Batch
	if batch < 1 {
		batch = uint32(runtime.NumCPU())
	}
	// cylinders
	for i := 0; i < len(engine.cylinders); i++ {
		cylinder, cylinderErr := newIOURingCylinder(entries, param, batch)
		if cylinderErr != nil {
			panic(fmt.Errorf("aio: engine start failed, %v", cylinderErr))
			return
		}
		engine.cylinders[i] = cylinder

	}
	for _, cylinder := range engine.cylinders {
		go cylinder.Loop(engine.markCylinderLoop, engine.markCylinderStop)
	}
}

func (engine *Engine) Stop() {
	runtime.SetFinalizer(engine, nil)

	for _, cylinder := range engine.cylinders {
		cylinder.Stop()
	}
	engine.wg.Wait()
}

func newIOURingCylinder(entries uint32, param *IOURingSetupParam, batch uint32) (cylinder Cylinder, err error) {
	// setup
	ring, ringErr := NewIOURing(entries, param)
	if ringErr != nil {
		err = ringErr
		return
	}
	// cylinder
	cylinder = &IOURingCylinder{
		ring:  ring,
		batch: batch,
	}
	return
}

// todo IOURing 单独结构， cylinder 管理 IOURing （batch，loop）
// 关闭 loop 可以试一试 注册一个 op，然后当。。也不要，engine 不用 rxp，就rxp结束后必然是空的，就直接关闭
type IOURingCylinder struct {
	ring   *IOURing
	batch  uint32
	stopCh chan struct{}
}

func (cylinder *IOURingCylinder) Fd() int {
	return cylinder.ring.fd
}

func (cylinder *IOURingCylinder) Loop(beg func(), end func()) {
	beg()
	defer end()

	ring := cylinder.ring
	cqes := make([]*CompletionQueueEvent, cylinder.batch)
	stopped := false
	for {
		select {
		case <-cylinder.stopCh:
			stopped = true
			break
		default:
			peeked := ring.PeekBatchCQE(cqes)
			for i := uint32(0); i < peeked; i++ {
				cqe := cqes[i]
				// get op from userdata
				op := (*Operator)(unsafe.Pointer(uintptr(cqe.UserData)))
				// todo handle canceled
				// todo 如果不返回的话，那么这里都是正确的结果，则改超时，cancel的同时 直接 completion（）
				// timeout
				if timer := op.timer; timer != nil {
					timer.Done()
					putOperatorTimer(timer)
					op.timer = nil
				}
				// complete op
				if completion := op.completion; completion != nil {
					completion(int(cqe.Res), op, nil)
					op.completion = nil
				}
				op.callback = nil
				runtime.KeepAlive(op)
			}
			break
		}
		if stopped {
			break
		}
	}
}

func (cylinder *IOURingCylinder) Stop() {
	// break loop
	close(cylinder.stopCh)
	// queue exit
	cylinder.ring.queueExit()
	return
}

func (cylinder *IOURingCylinder) Up() {}

func (cylinder *IOURingCylinder) Down() {}

func (cylinder *IOURingCylinder) Actives() int64 {
	return int64(cylinder.ring.sqSpaceLeft())
}

func NewIOURing(entries uint32, param *IOURingSetupParam) (*IOURing, error) {
	ring := &IOURing{}
	err := ring.setup(entries, param)
	if err != nil {
		return nil, err
	}
	return ring, nil
}

type IOURing struct {
	sq       *SubmissionQueue
	cq       *CompletionQueue
	flags    uint32
	fd       int
	features uint32
	enterFd  int
	intFlags uint8
	// nolint: unused
	pad [3]uint8
	// nolint: unused
	pad2 uint32
}

func (ring *IOURing) GetSQE() *SubmissionQueueEntry {
	sq := ring.sq
	var head, next uint32
	var shift int

	if ring.flags&SetupSQE128 != 0 {
		shift = 1
	}
	head = atomic.LoadUint32(sq.head)
	next = sq.sqeTail + 1
	if next-head <= *sq.ringEntries {
		sqe := (*SubmissionQueueEntry)(
			unsafe.Add(unsafe.Pointer(ring.sq.sqes),
				uintptr((sq.sqeTail&*sq.ringMask)<<shift)*unsafe.Sizeof(SubmissionQueueEntry{})),
		)
		sq.sqeTail = next

		return sqe
	}

	return nil
}

func (ring *IOURing) Submit() (uint, error) {
	submitted := ring.flushSQ()
	cqNeedsEnter := ring.cqNeedsEnter()

	var flags uint32
	var ret uint
	var err error

	flags = 0
	if ring.sqNeedsEnter(submitted, &flags) || cqNeedsEnter {
		if cqNeedsEnter {
			flags |= EnterGetEvents
		}
		ret, err = ring.Enter(submitted, 0, flags, nil)
		if err != nil {
			return 0, err
		}
	} else {
		ret = uint(submitted)
	}

	return ret, nil
}

func (ring *IOURing) PeekBatchCQE(cqes []*CompletionQueueEvent) (peeked uint32) {
	cqesLen := uint32(len(cqes))
	if cqesLen == 0 {
		return
	}
	overflowChecked := false
	shift := 0
	if ring.flags&SetupCQE32 != 0 {
		shift = 1
	}
	for {
		ready := ring.cqReady()
		if ready != 0 {
			head := *ring.cq.head
			mask := *ring.cq.ringMask
			last := head + cqesLen
			if cqesLen > ready {
				peeked = ready
			} else {
				peeked = cqesLen
			}
			for i := 0; head != last; head, i = head+1, i+1 {
				cqes[i] = (*CompletionQueueEvent)(
					unsafe.Add(
						unsafe.Pointer(ring.cq.cqes),
						uintptr((head&mask)<<shift)*unsafe.Sizeof(CompletionQueueEvent{}),
					),
				)
			}
			break
		}

		if overflowChecked {
			break
		}

		if ring.cqNeedsFlush() {
			_, _ = ring.getEvents()
			overflowChecked = true
			continue
		}
	}

	return
}

func (ring *IOURing) CQAdvance(n uint32) {
	atomic.StoreUint32(ring.cq.head, *ring.cq.head+n)
}

func (ring *IOURing) Exit() {
	ring.queueExit()
}

const (
	sysSetup    = 425
	sysEnter    = 426
	sysRegister = 427
)

func (ring *IOURing) setup(entries uint32, param *IOURingSetupParam) (err error) {
	fdPtr, _, errno := syscall.Syscall(sysSetup, uintptr(entries), uintptr(unsafe.Pointer(param)), 0)
	if errno != 0 {
		err = errno
		return
	}

	fd := int(fdPtr)

	err = ring.queueMmap(fd, param)
	if err != nil {
		_ = syscall.Close(fd)
		return
	}

	sqEntries := *ring.sq.ringEntries
	for index := uint32(0); index < sqEntries; index++ {
		*(*uint32)(
			unsafe.Add(unsafe.Pointer(ring.sq.array),
				index*uint32(unsafe.Sizeof(uint32(0))))) = index
	}

	ring.features = param.Features
	ring.flags = param.Flags
	ring.enterFd = fd

	ring.fd = fd
	return
}

func (ring *IOURing) queueExit() {
	sq := ring.sq
	if sq.ringSize == 0 {
		sqeSize := unsafe.Sizeof(SubmissionQueueEntry{})
		if ring.flags&SetupSQE128 != 0 {
			sqeSize += 64
		}
		_ = munmap(uintptr(unsafe.Pointer(sq.sqes)), sqeSize*uintptr(*sq.ringEntries))

	} else {
		_ = munmap(uintptr(unsafe.Pointer(sq.sqes)), uintptr(*sq.ringEntries)*unsafe.Sizeof(SubmissionQueueEntry{}))
	}

	ring.queueMumap()
	if fd := ring.fd; fd != 0 {
		_ = syscall.Close(fd)
	}
}

func (ring *IOURing) queueMmap(fd int, param *IOURingSetupParam) (err error) {

	size := unsafe.Sizeof(CompletionQueueEvent{})
	if param.Flags&SetupCQE32 != 0 {
		size += unsafe.Sizeof(CompletionQueueEvent{})
	}

	sq := ring.sq
	cq := ring.cq

	sq.ringSize = uint(uintptr(param.SQOff.Array) + uintptr(param.SQEntries)*unsafe.Sizeof(uint32(0)))
	cq.ringSize = uint(uintptr(param.CQOff.CQes) + uintptr(param.CQEntries)*size)

	if param.Features&FeatSingleMMap != 0 {
		if cq.ringSize > sq.ringSize {
			sq.ringSize = cq.ringSize
		}
		cq.ringSize = sq.ringSize
	}

	ringPtr, mmapErr := mmap(0, uintptr(sq.ringSize), syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED|syscall.MAP_POPULATE, fd,
		int64(offSQRing))
	if mmapErr != nil {
		err = mmapErr
		return
	}
	sq.ringPtr = unsafe.Pointer(ringPtr)

	if param.Features&FeatSingleMMap != 0 {
		cq.ringPtr = sq.ringPtr
	} else {
		ringPtr, mmapErr = mmap(0, uintptr(cq.ringSize), syscall.PROT_READ|syscall.PROT_WRITE,
			syscall.MAP_SHARED|syscall.MAP_POPULATE, fd,
			int64(offCQRing))
		if mmapErr != nil {
			cq.ringPtr = nil
			ring.queueMumap()
			err = mmapErr
			return
		}
		cq.ringPtr = unsafe.Pointer(ringPtr)
	}

	size = unsafe.Sizeof(SubmissionQueueEntry{})
	if param.Flags&SetupSQE128 != 0 {
		size += 64
	}
	ringPtr, mmapErr = mmap(0, size*uintptr(param.SQEntries), syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED|syscall.MAP_POPULATE, fd, int64(offSQEs))
	if mmapErr != nil {
		ring.queueMumap()
		err = mmapErr
		return
	}
	sq.sqes = (*SubmissionQueueEntry)(unsafe.Pointer(ringPtr))

	// setup queue
	sq.head = (*uint32)(unsafe.Pointer(uintptr(sq.ringPtr) + uintptr(param.SQOff.Head)))
	sq.tail = (*uint32)(unsafe.Pointer(uintptr(sq.ringPtr) + uintptr(param.SQOff.Tail)))
	sq.ringMask = (*uint32)(unsafe.Pointer(uintptr(sq.ringPtr) + uintptr(param.SQOff.RingMask)))
	sq.ringEntries = (*uint32)(unsafe.Pointer(uintptr(sq.ringPtr) + uintptr(param.SQOff.RingEntries)))
	sq.flags = (*uint32)(unsafe.Pointer(uintptr(sq.ringPtr) + uintptr(param.SQOff.Flags)))
	sq.dropped = (*uint32)(unsafe.Pointer(uintptr(sq.ringPtr) + uintptr(param.SQOff.Dropped)))
	sq.array = (*uint32)(unsafe.Pointer(uintptr(sq.ringPtr) + uintptr(param.SQOff.Array)))

	cq.head = (*uint32)(unsafe.Pointer(uintptr(cq.ringPtr) + uintptr(param.CQOff.Head)))
	cq.tail = (*uint32)(unsafe.Pointer(uintptr(cq.ringPtr) + uintptr(param.CQOff.Tail)))
	cq.ringMask = (*uint32)(unsafe.Pointer(uintptr(cq.ringPtr) + uintptr(param.CQOff.RingMask)))
	cq.ringEntries = (*uint32)(unsafe.Pointer(uintptr(cq.ringPtr) + uintptr(param.CQOff.RingEntries)))
	cq.overflow = (*uint32)(unsafe.Pointer(uintptr(cq.ringPtr) + uintptr(param.CQOff.Overflow)))
	cq.cqes = (*CompletionQueueEvent)(unsafe.Pointer(uintptr(cq.ringPtr) + uintptr(param.CQOff.CQes)))
	if param.CQOff.Flags != 0 {
		cq.flags = (*uint32)(unsafe.Pointer(uintptr(cq.ringPtr) + uintptr(param.CQOff.Flags)))
	}
	return
}

func (ring *IOURing) queueMumap() {
	sq := ring.sq
	if sq.ringSize > 0 {
		_ = munmap(uintptr(sq.ringPtr), uintptr(sq.ringSize))
	}
	cq := ring.cq
	if uintptr(cq.ringPtr) != 0 && cq.ringSize > 0 && cq.ringPtr != sq.ringPtr {
		_ = munmap(uintptr(cq.ringPtr), uintptr(cq.ringSize))
	}
	return
}

func (ring *IOURing) sqNeedsEnter(submit uint32, flags *uint32) bool {
	if submit == 0 {
		return false
	}

	if (ring.flags & SetupSQPoll) == 0 {
		return true
	}

	if atomic.LoadUint32(ring.sq.flags)&SQNeedWakeup != 0 {
		*flags |= EnterSQWakeup

		return true
	}

	return false
}

func (ring *IOURing) flushSQ() uint32 {
	sq := ring.sq
	tail := sq.sqeTail

	if sq.sqeHead != tail {
		sq.sqeHead = tail
		atomic.StoreUint32(sq.tail, tail)
	}

	return tail - atomic.LoadUint32(sq.head)
}

func (ring *IOURing) sqReady() uint32 {
	khead := *ring.sq.head

	if ring.flags&SetupSQPoll != 0 {
		khead = atomic.LoadUint32(ring.sq.head)
	}

	return ring.sq.sqeTail - khead
}

func (ring *IOURing) sqSpaceLeft() uint32 {
	return *ring.sq.ringEntries - ring.sqReady()
}

func (ring *IOURing) cqReady() uint32 {
	return atomic.LoadUint32(ring.cq.tail) - *ring.cq.head
}

func (ring *IOURing) getEvents() (uint, error) {
	flags := EnterGetEvents
	return ring.Enter(0, 0, flags, nil)
}

func (ring *IOURing) cqeShift() uint32 {
	if ring.flags&SetupCQE32 != 0 {
		return 1
	}

	return 0
}

func (ring *IOURing) cqeIndex(ptr, mask uint32) uintptr {
	return uintptr((ptr & mask) << ring.cqeShift())
}

func (ring *IOURing) cqNeedsEnter() bool {
	return (ring.flags&SetupIOPoll) != 0 || ring.cqNeedsFlush()
}

func (ring *IOURing) cqNeedsFlush() bool {
	return atomic.LoadUint32(ring.sq.flags)&(SQCQOverflow|SQTaskRun) != 0
}

const (
	nSig                    = 65
	szDivider               = 8
	registerRingFdOffset    = uint32(4294967295)
	regIOWQMaxWorkersNrArgs = 2
)

func (ring *IOURing) Enter(submitted uint32, waitNr uint32, flags uint32, sig unsafe.Pointer) (uint, error) {
	return ring.Enter2(submitted, waitNr, flags, sig, nSig/szDivider)
}

func (ring *IOURing) Enter2(submitted uint32, waitNr uint32, flags uint32, sig unsafe.Pointer, size int) (uint, error) {
	consumed, _, errno := syscall.Syscall6(
		sysEnter,
		uintptr(ring.enterFd),
		uintptr(submitted),
		uintptr(waitNr),
		uintptr(flags),
		uintptr(sig),
		uintptr(size),
	)
	if errno > 0 {
		return 0, errno
	}
	return uint(consumed), nil
}

type IOURingSettings struct {
	Entries uint32
	Param   IOURingSetupParam
	Batch   uint32
}

type IOURingSetupParam struct {
	SQEntries uint32
	// CQEntries
	// 默认情况下，CQ 环的条目数将是SQ环条目数的两倍。
	// 这对于常规文件或存储工作负载来说是足够的，但对于网络工作负载来说可能太小。
	// SQ环条目没有对环可以支持的正在进行的请求数量施加限制，它只是限制了一次（批处理）可以提交给内核的数量。
	// 如果CQ环溢出，例如，在应用程序可以获取之前，生成的条目比环中适合的条目多，
	// 那么如果内核支持 IORING_FEAT_NODROP ，则环将进入CQ环溢流状态。
	// 否则，它将删除CQE，并在结构 io_uring 中随着删除的CQE数量递增 cq.koverflow。
	// 溢出状态由SQ环标志中设置的 IORING_SQ_CQ_overflow 表示。
	// 除非内核耗尽可用内存，否则条目不会被删除，但这是一条慢得多的完成路径，会减慢请求处理速度。
	// 因此，应避免使用CQ环，CQ环的大小应适合工作负载。
	// 在结构 io_uring_params 中设置 cq_entrys 将告诉内核为cq环分配这么多条目，而与给定条目中的SQ环大小无关。
	// 如果该值不是2的幂，则将四舍五入到最接近的2的幂。
	CQEntries    uint32
	Flags        uint32
	SQThreadCPU  uint32
	SQThreadIdle uint32
	Features     uint32
	WqFd         uint32
	Resv         [3]uint32
	SQOff        SQRingOffsets
	CQOff        CQRingOffsets
}

type SQRingOffsets struct {
	Head        uint32
	Tail        uint32
	RingMask    uint32
	RingEntries uint32
	Flags       uint32
	Dropped     uint32
	Array       uint32
	Resv1       uint32
	UserAddr    uint64
}

type CQRingOffsets struct {
	Head        uint32
	Tail        uint32
	RingMask    uint32
	RingEntries uint32
	Overflow    uint32
	CQes        uint32
	Flags       uint32
	Resv1       uint32
	UserAddr    uint64
}

type SubmissionQueue struct {
	head        *uint32
	tail        *uint32
	ringMask    *uint32
	ringEntries *uint32
	flags       *uint32
	dropped     *uint32
	array       *uint32
	sqes        *SubmissionQueueEntry

	ringSize uint
	ringPtr  unsafe.Pointer

	sqeHead uint32
	sqeTail uint32

	// nolint: unused
	pad [2]uint32
}

type SubmissionQueueEntry struct {
	OpCode      uint8
	Flags       uint8
	IoPrio      uint16
	Fd          int32
	Off         uint64
	Addr        uint64
	Len         uint32
	OpcodeFlags uint32
	UserData    uint64
	BufIG       uint16
	Personality uint16
	SpliceFdIn  int32
	Addr3       uint64
	_pad2       [1]uint64
	// TODO: add __u8	cmd[0];
}

type CompletionQueue struct {
	head        *uint32
	tail        *uint32
	ringMask    *uint32
	ringEntries *uint32
	flags       *uint32
	overflow    *uint32
	cqes        *CompletionQueueEvent
	ringSize    uint
	ringPtr     unsafe.Pointer
	pad         [2]uint32
}

type CompletionQueueEvent struct {
	UserData uint64
	Res      int32
	Flags    uint32
	// FIXME
	// 	__u64 big_cqe[];
}

const (
	offSQRing    uint64 = 0
	offCQRing    uint64 = 0x8000000
	offSQEs      uint64 = 0x10000000
	offPbufRing  uint64 = 0x80000000
	offPbufShift uint64 = 16
	offMmapMask  uint64 = 0xf8000000
)

// setup and features
// https://manpages.debian.org/unstable/liburing-dev/io_uring_setup.2.en.html
const (
	// SetupIOPoll
	// 执行繁忙等待 I/O 完成，而不是通过异步 IRQ（中断请求）获取通知。文件系统（如有）和块设备必须支持轮询，这样才能正常工作。
	// 忙时（Busy-waiting）可提供较低的延迟，但可能比中断驱动的 I/O 消耗更多的 CPU 资源。
	// 目前，该功能仅适用于使用 O_DIRECT 标志打开的文件描述符。
	// 向轮询上下文提交读或写操作时，应用程序必须调用 io_uring_enter(2) 来轮询 CQ 环上的完成情况。在 io_uring 实例上混合匹配轮询和非轮询 I/O 是非法的。
	// 目前这只适用于存储设备，而且存储设备必须配置为轮询。如何配置取决于相关设备的类型。
	// 对于 NVMe 设备，必须加载 nvme 驱动程序，并将 poll_queues 参数设置为所需的轮询队列数。
	// 如果轮询队列的数量少于在线 CPU 线程的数量，系统中的 CPU 将适当共享轮询队列。
	SetupIOPoll uint32 = 1 << iota
	// SetupSQPoll
	// 指定该标志后，将创建一个内核线程来执行提交队列轮询。以这种方式配置的 io_uring 实例能让应用程序在不切换内核上下文的情况下发出 I/O。通过使用提交队列填写新的提交队列条目，并观察完成队列上的完成情况，应用程序可以在不执行单个系统调用的情况下提交和获取 I/O。
	// 如果内核线程空闲时间超过 sq_thread_idle 毫秒，就会设置结构 io_sq_ring 的 flags 字段中的 IORING_SQ_NEED_WAKEUP 位。出现这种情况时，应用程序必须调用 io_uring_enter(2) 来唤醒内核线程。如果 I/O 一直处于繁忙状态，内核线程将永远不会休眠。使用此功能的应用程序需要用以下代码序列来保护 io_uring_enter(2) 调用：
	// unsigned flags = atomic_load_relaxed(sq_ring->flags);
	// if flags & IORING_SQ_NEED_WAKEUP {
	//    io_uring_enter(fd, 0, 0, IORING_ENTER_SQ_WAKEUP);
	// }
	// 其中，sq_ring 是使用下述结构 io_sqring_offsets 设置的提交队列环。
	//
	// 请注意，在使用 IORING_SETUP_SQPOLL 进行环形设置时，千万不要直接调用 io_uring_enter(2) 系统调用。这通常由 liburing 的 io_uring_submit(3) 函数负责。它会自动判断你是否在使用轮询模式，并在你的程序需要调用 io_uring_enter(2) 时进行处理，无需你费心。
	// 在 Linux 内核 5.11 版本之前，要成功使用这一功能，应用程序必须使用 IORING_REGISTER_FILES 操作码通过 io_uring_register(2) 注册一组用于 IO 的文件。否则，提交的 IO 将出现 EBADF 错误。可以通过 IORING_FEAT_SQPOLL_NONFIXED 功能标志检测该功能是否存在。在 5.11 及更高版本中，使用此功能不再需要注册文件。如果用户具有 CAP_SYS_NICE 功能，5.11 还允许以非 root 身份使用此功能。在 5.13 中，这一要求也被放宽，在较新的内核中，SQPOLL 不需要特殊权限。某些比 5.13 版本更早的稳定内核也可能支持非特权 SQPOLL。
	SetupSQPoll
	// SetupSQAff
	// 如果指定了这个标志，那么轮询线程将绑定到结构 io_uring_params 的 sq_thread_cpu 字段中设置的 cpu。该标志只有在指定 IORING_SETUP_SQPOLL 时才有意义。当 cgroup 设置 cpuset.cpus 发生变化时（通常是在容器环境中），绑定的 cpu 集也会发生变化。
	SetupSQAff
	// SetupCQSize
	// 使用 struct io_uring_params.cq_entries 条目创建完成队列。值必须大于条目数，并可四舍五入为下一个 2 的幂次。
	SetupCQSize
	// SetupClamp
	// 如果指定了该标志，且条目数超过 IORING_MAX_ENTRIES，那么条目数将被箝位在 IORING_MAX_ENTRIES。如果设置了标志 IORING_SETUP_CQSIZE，且 struct io_uring_params.cq_entries 的值超过了 IORING_MAX_CQ_ENTRIES，则将以 IORING_MAX_CQ_ENTRIES 的值箝位。
	SetupClamp
	// SetupAttachWQ
	// 设置该标志时，应同时将 struct io_uring_params.wq_fd 设置为现有的 io_uring ring 文件描述符。设置后，创建的 io_uring 实例将共享指定 io_uring ring 的异步工作线程后端，而不是创建一个新的独立线程池。此外，如果设置了 IORING_SETUP_SQPOLL，还将共享 sq 轮询线程。
	SetupAttachWQ
	// SetupRDisabled
	// 如果指定了该标记，io_uring 环将处于禁用状态。在这种状态下，可以注册限制，但不允许提交。有关如何启用环的详细信息，请参见 io_uring_register(2)。自 5.10 版起可用。
	SetupRDisabled
	// SetupSubmitAll
	// 通常情况下，如果其中一个请求出现错误，io_uring 就会停止提交一批请求。如果一个请求在提交过程中出错，这可能会导致提交的请求少于预期。如果在创建环时使用了此标记，那么即使在提交请求时遇到错误，io_uring_enter(2) 也会继续提交请求。无论在创建环时是否设置了此标记，都会为出错的请求发布 CQE，唯一的区别在于当发现错误时，提交序列是停止还是继续。自 5.18 版起可用。
	SetupSubmitAll
	// SetupCoopTaskrun
	// 默认情况下，当有完成事件发生时，io_uring 会中断在用户空间运行的任务。
	// 这是为了确保完成任务及时运行。
	// 对于很多用例来说，这样做有些矫枉过正，会导致性能下降，包括用于中断的处理器间中断、内核/用户转换、对任务用户空间活动的无谓中断，以及如果完成事件来得太快，批处理能力下降。
	// 大多数应用程序不需要强制中断，因为事件会在任何内核/用户转换时得到处理。
	// 【例外情况是，应用程序使用多个线程在同一环上运行，在这种情况下，等待完成的应用程序并不是提交完成的应用程序。】
	// 【对于大多数其他使用情况，设置此标志将提高性能。自 5.19 版起可用。】
	SetupCoopTaskrun
	// SetupTaskrunFlag
	// 与 IORING_SETUP_COOP_TASKRUN 结合使用，它提供了一个标志 IORING_SQ_TASKRUN，
	// 每当有应该处理的完成等待时，它就会在 SQ 环标志中被设置。即使在执行 io_uring_peek_cqe(3) 时，
	// liburing 也会检查该标志，并进入内核处理它们，应用程序也可以这样做。
	// 这使得 IORING_SETUP_TASKRUN_FLAG 可以安全使用，即使应用程序依赖于 CQ 环上的偷看式操作来查看是否有任何待收获。自 5.19 版起可用。
	SetupTaskrunFlag
	// SetupSQE128
	// 如果设置了该选项，io_uring 将使用 128 字节的 SQE，而不是正常的 64 字节大小的 SQE。这是使用某些请求类型的要求，截至 5.19 版，只有用于 NVMe 直通的 IORING_OP_URING_CMD 直通命令需要使用此功能。自 5.19 版起可用。
	SetupSQE128
	// SetupCQE32
	// 如果设置了该选项，io_uring 将使用 32 字节的 CQE，而非通常的 16 字节大小。这是使用某些请求类型的要求，截至 5.19 版，只有用于 NVMe 直通的 IORING_OP_URING_CMD 直通命令需要使用此功能。自 5.19 版起可用。
	SetupCQE32
	// SetupSingleIssuer
	// 提示内核只有一个任务（或线程）提交请求，用于内部优化。提交任务要么是创建环的任务，要么是通过 io_uring_register(2) 启用环的任务（如果指定了 IORING_SETUP_R_DISABLED）。内核会强制执行这一规则，如果违反限制，会以 -EEXIST 失败请求。需要注意的是，当设置了 IORING_SETUP_SQPOLL 时，轮询任务将被视为代表用户空间完成所有提交工作，因此无论有多少用户空间任务执行 io_uring_enter(2)，轮询任务都会遵守该规则。自 6.0 版起可用。
	SetupSingleIssuer
	// SetupDeferTaskrun
	// 默认情况下，io_uring 会在任何系统调用或线程中断结束时处理所有未完成的工作。这可能会延迟应用程序取得其他进展。设置该标志将提示 io_uring 将工作推迟到设置了 IORING_ENTER_GETEVENTS 标志的 io_uring_enter(2) 调用。这样，应用程序就可以在处理完成之前请求运行工作。该标志要求设置 IORING_SETUP_SINGLE_ISSUER 标志，并强制要求从提交请求的同一线程调用 io_uring_enter(2)。请注意，如果设置了该标记，应用程序就有责任定期触发工作（例如通过任何 CQE 等待函数），否则可能无法交付完成。自 6.1 版起可用。
	SetupDeferTaskrun
	// SetupNoMmap
	// 默认情况下，io_uring 会分配内核内存，调用者必须随后使用 mmap(2)。如果设置了该标记，io_uring 将使用调用者分配的缓冲区；p->cq_off.user_addr 必须指向 sq/cq ring 的内存，p->sq_off.user_addr 必须指向 sqes 的内存。每次分配的内存必须是连续的。通常情况下，调用者应使用 mmap(2) 分配大页面来分配这些内存。如果设置了此标记，那么随后尝试 mmap(2) io_uring 文件描述符的操作将失败。自 6.5 版起可用。
	_SetupNoMmap
	// SetupRegisteredFdOnly
	// 如果设置了这个标志，io_uring 将注册环形文件描述符，并返回已注册的描述符索引，而不会分配一个未注册的文件描述符。调用者在调用 io_uring_register(2) 时需要使用 IORING_REGISTER_USE_REGISTERED_RING。该标记只有在与 IORING_SETUP_NO_MMAP 同时使用时才有意义，后者也需要设置。自 6.5 版起可用。
	_SetupRegisteredFdOnly
	// SetupNoSQArray
	// 如果设置了该标志，提交队列中的条目将按顺序提交，并在到达队列末尾后绕到第一个条目。换句话说，将不再通过提交条目数组进行间接处理，而是直接通过提交队列尾部和它所代表的索引范围（队列大小的模数）对队列进行索引。随后，用户不应映射提交队列条目数组，结构 io_sqring_offsets 中的相应偏移量将被设置为零。自 6.6 版起可用。
	// 如果没有指定标志，io_uring 实例将设置为中断驱动 I/O。可以使用 io_uring_enter(2) 提交 I/O，并通过轮询完成队列获取 I/O。
	//
	// resv 数组必须初始化为零。
	//
	// features 由内核填写，指定当前内核版本支持的各种功能。
	SetupNoSQArray
)

const (
	// FeatSingleMMap
	// 如果设置了该标志，则只需调用一次 mmap(2)，即可映射两个 SQ 和 CQ 环。SQE 仍需单独分配。这样，所需的 mmap(2) 调用次数就从三次减少到两次。自内核 5.4 起可用。
	FeatSingleMMap uint32 = 1 << iota
	// FeatNoDrop
	// 如果设置了这个标志，io_uring 就几乎不会丢弃完成事件。只有当内核内存耗尽时，才会发生丢弃事件，在这种情况下，你会遇到比丢失事件更严重的问题。无论如何，你的应用程序和其他程序都可能会被 OOM 杀掉。如果发生了完成事件，而 CQ 环已满，内核会在内部存储该事件，直到 CQ 环有空间容纳更多条目。在早期内核中，如果出现这种溢出情况，尝试提交更多 IO 时，如果无法将溢出的事件刷新到 CQ 环，就会出现 -EBUSY 错误值而失败。如果出现这种情况，应用程序必须从 CQ 环中获取事件，并再次尝试提交。如果内核内部没有空闲内存来存储事件，那么 cqring 上的溢出值就会增加。自内核 5.5 起可用。此外，io_uring_enter(2) 还会在下一次休眠等待完成时返回 -EBADR（自内核 5.19 起）。
	FeatNoDrop
	// FeatSubmitStable
	// 如果设置了该标志，应用程序就可以确定，当内核消耗 SQE 时，任何用于异步卸载的数据都已消耗完毕。自内核 5.5 起可用。
	FeatSubmitStable
	// FeatRWCurPos
	// 如果设置了这个标志，应用程序就可以在 IORING_OP_{READV,WRITEV}、IORING_OP_{READ,WRITE}_FIXED 和 IORING_OP_{READ,WRITE} 中指定偏移量 == -1 表示当前文件位置，其行为与偏移量 == -1 的 preadv2(2) 和 pwritev2(2) 类似。 它将使用（并更新）当前文件位置。这显然需要注意的是，如果应用程序在运行过程中进行了多次读取或写入，那么最终结果将不尽如人意。这与线程共享文件描述符并使用当前文件位置进行 IO 的情况类似。自内核 5.6 起可用。
	FeatRWCurPos
	// FeatCurPersonality
	// 如果设置了这个标志，那么 io_uring 将保证同步和异步执行请求时，都使用调用 io_uring_enter(2) 对请求进行排队的任务的凭据。如果未设置该标记，则会使用最初注册 io_uring 的任务的凭据发出请求。如果只有一个任务在使用一个环，那么这个标志并不重要，因为凭据始终是相同的。请注意，这是默认行为，任务仍然可以通过 io_uring_register(2) 以 IORING_REGISTER_PERSONALITY 注册不同的个性，并在 sqe 中指定要使用的个性。自内核 5.6 起可用。
	FeatCurPersonality
	// FeatFastPoll
	// 如果设置了这个标志，那么 io_uring 将支持使用内部轮询机制来驱动数据/空间就绪。这意味着无法读取或写入文件数据的请求不再需要交由异步线程处理，而是会在文件就绪时开始运行。这类似于在用户空间进行轮询 + 读/写操作，但无需这样做。如果设置了该标记，等待空间/数据的请求就不会阻塞线程，从而减少了资源消耗。自内核 5.7 起可用。
	FeatFastPoll
	// FeatPoll32Bits
	// 如果设置了该标志，IORING_OP_POLL_ADD 命令将接受全部 32 位的基于 epoll 的标志。最值得注意的是 EPOLLEXCLUSIVE，它允许独占（唤醒单个等待者）行为。自内核 5.9 起可用。
	FeatPoll32Bits
	// FeatSQPollNonfixed
	// 如果设置了该标志，IORING_SETUP_SQPOLL 功能就不再需要使用固定文件。任何普通文件描述符都可用于 IO 命令，无需注册。自内核 5.11 起可用。
	FeatSQPollNonfixed
	// FeatExtArg
	// 如果设置了这个标志，io_uring_enter(2) 系统调用就支持传递一个扩展参数，而不仅仅是早期内核的 sigset_t。这个扩展参数的类型是 struct io_uring_getevents_arg，允许调用者同时传递 sigset_t 和超时参数，以等待事件发生。结构布局如下
	//结构 io_uring_getevents_arg {
	//
	//
	//    __u64 sigmask；
	//
	//
	//    __u32 sigmask_sz；
	//
	//
	//    __u32 pad；
	//
	//
	//    __u64 ts；
	//};
	//如果在 enter 系统调用的标志中设置了 IORING_ENTER_EXT_ARG，则必须传入指向该结构的指针。自内核 5.11 起可用。
	FeatExtArg
	// FeatNativeWorkers
	// 如果设置了这个标志，那么 io_uring 将使用本地 Worker 作为异步助手。以前的内核使用的内核线程会假定原始 io_uring 拥有任务的身份，但以后的内核会主动创建看起来更像普通进程的线程。自内核 5.12 起可用。
	FeatNativeWorkers
	// FeatRcrcTags
	// 如果设置了这个标志，那么 io_uring 将支持与固定文件和缓冲区相关的各种功能。尤其是，它表明已注册的缓冲区可以就地更新，而在此之前，必须先取消注册整个缓冲区。自内核 5.13 起可用。
	FeatRcrcTags
	// FeatCQESkip
	// 如果设置了该标志，io_uring 就支持在提交的 SQE 中设置 IOSQE_CQE_SKIP_SUCCESS，表明如果正常执行，就不会为该 SQE 生成 CQE。如果在处理 SQE 时发生错误，仍会生成带有相应错误值的 CQE。自内核 5.17 起可用。
	FeatCQESkip
	// FeatLinkedFile
	// 如果设置了这个标志，那么 io_uring 将支持为有依赖关系的 SQE 合理分配文件。例如，如果使用 IOSQE_IO_LINK 提交了一连串 SQE，那么没有该标志的内核将为每个链接预先准备文件。如果前一个链接打开了一个已知索引的文件，例如使用直接描述符打开或接受，那么文件分配就需要在执行该 SQE 后进行。如果设置了该标志，内核将推迟文件分配，直到开始执行给定请求。自内核 5.17 起可用。
	FeatLinkedFile
	// FeatRegRegRing
	// 如果设置了该标志，则 io_uring 支持通过 IORING_REGISTER_USE_REGISTERED_RING，使用注册环 fd 调用 io_uring_register(2)。自内核 6.3 起可用。
	FeatRegRegRing
	FeatRecvSendBundle
	// FeatMinTimeout
	// 如果设置了该标志，则 io_uring 支持传递最小批处理等待超时。详情请参见 io_uring_submit_and_wait_min_timeout(3) 。
	FeatMinTimeout
)

const (
	EnterGetEvents uint32 = 1 << iota
	EnterSQWakeup
	EnterSQWait
	EnterExtArg
	EnterRegisteredRing
)

const (
	SQNeedWakeup uint32 = 1 << iota
	SQCQOverflow
	SQTaskRun
)
