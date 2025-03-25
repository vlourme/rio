//go:build linux

package liburing

import (
	"strings"
)

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
	// 它会使用很多cpu资源。
	// 指定该标志后，将创建一个内核线程来执行提交队列轮询。以这种方式配置的 io_uring 实例能让应用程序在不切换内核上下文的情况下发出 I/O。
	// 通过使用提交队列填写新的提交队列条目，并观察完成队列上的完成情况，应用程序可以在不执行单个系统调用的情况下提交和获取 I/O。
	// 如果内核线程空闲时间超过 sq_thread_idle 毫秒，就会设置结构 io_sq_ring 的 flags 字段中的 IORING_SQ_NEED_WAKEUP 位。
	// 出现这种情况时，应用程序必须调用 io_uring_enter(2) 来唤醒内核线程。
	// 如果 I/O 一直处于繁忙状态，内核线程将永远不会休眠。使用此功能的应用程序需要用以下代码序列来保护 io_uring_enter(2) 调用：
	// unsigned flags = atomic_load_relaxed(sq_ring->flags);
	// if flags & IORING_SQ_NEED_WAKEUP {
	//    io_uring_enter(fd, 0, 0, IORING_ENTER_SQ_WAKEUP);
	// }
	// 其中，sq_ring 是使用下述结构 io_sqring_offsets 设置的提交队列环。
	//
	// 请注意，在使用 IORING_SETUP_SQPOLL 进行环形设置时，千万不要直接调用 io_uring_enter(2) 系统调用。
	// 这通常由 liburing 的 io_uring_submit(3) 函数负责。它会自动判断你是否在使用轮询模式，并在你的程序需要调用 io_uring_enter(2) 时进行处理，无需你费心。
	// 在 Linux 内核 5.11 版本之前，要成功使用这一功能，应用程序必须使用 IORING_REGISTER_FILES 操作码通过 io_uring_register(2) 注册一组用于 IO 的文件。
	// 否则，提交的 IO 将出现 EBADF 错误。可以通过 IORING_FEAT_SQPOLL_NONFIXED 功能标志检测该功能是否存在。
	// 在 5.11 及更高版本中，使用此功能不再需要注册文件。
	// 如果用户具有 CAP_SYS_NICE 功能，5.11 还允许以非 root 身份使用此功能。
	// 在 5.13 中，这一要求也被放宽，在较新的内核中，SQPOLL 不需要特殊权限。
	// 某些比 5.13 版本更早的稳定内核也可能支持非特权 SQPOLL。
	SetupSQPoll
	// SetupSQAff
	// 如果指定了这个标志，那么轮询线程将绑定到结构 io_uring_params 的 sq_thread_cpu 字段中设置的 cpu。
	// 该标志只有在指定 IORING_SETUP_SQPOLL 时才有意义。
	// 当 cgroup 设置 cpuset.cpus 发生变化时（通常是在容器环境中），绑定的 cpu 集也会发生变化。
	SetupSQAff
	// SetupCQSize
	// 使用 struct io_uring_params.cq_entries 条目创建完成队列。值必须大于条目数，并可四舍五入为下一个 2 的幂次。
	SetupCQSize
	// SetupClamp
	// 如果指定了该标志，且条目数超过 IORING_MAX_ENTRIES，那么条目数将被箝位在 IORING_MAX_ENTRIES。
	// 如果设置了标志 IORING_SETUP_CQSIZE，且 struct io_uring_params.cq_entries 的值超过了 IORING_MAX_CQ_ENTRIES，则将以 IORING_MAX_CQ_ENTRIES 的值箝位。
	SetupClamp
	// SetupAttachWQ
	// 设置该标志时，应同时将 struct io_uring_params.wq_fd 设置为现有的 io_uring ring 文件描述符。
	// 设置后，创建的 io_uring 实例将共享指定 io_uring ring 的异步工作线程后端，而不是创建一个新的独立线程池。
	// 此外，如果设置了 IORING_SETUP_SQPOLL，还将共享 sq 轮询线程。
	SetupAttachWQ
	// SetupRDisabled
	// 如果指定了该标记，io_uring 环将处于禁用状态。在这种状态下，可以注册限制，但不允许提交。有关如何启用环的详细信息，请参见 io_uring_register(2)。自 5.10 版起可用。
	SetupRDisabled
	// SetupSubmitAll
	// 通常情况下，如果其中一个请求出现错误，io_uring 就会停止提交一批请求。
	// 如果一个请求在提交过程中出错，这可能会导致提交的请求少于预期。
	// 如果在创建环时使用了此标记，那么即使在提交请求时遇到错误，io_uring_enter(2) 也会继续提交请求。
	// 无论在创建环时是否设置了此标记，都会为出错的请求发布 CQE，唯一的区别在于当发现错误时，提交序列是停止还是继续。
	// 自 5.18 版起可用。
	SetupSubmitAll
	// SetupCoopTaskRun
	// 默认情况下，当有完成事件发生时，io_uring 会中断在用户空间运行的任务。
	// 这是为了确保完成任务及时运行。
	// 对于很多用例来说，这样做有些矫枉过正，会导致性能下降，包括用于中断的处理器间中断、内核/用户转换、对任务用户空间活动的无谓中断，以及如果完成事件来得太快，批处理能力下降。
	// 大多数应用程序不需要强制中断，因为事件会在任何内核/用户转换时得到处理。
	// 例外情况是，应用程序使用多个线程在同一环上运行，在这种情况下，等待完成的应用程序并不是提交完成的应用程序。
	// 对于大多数其他使用情况，设置此标志将提高性能。自 5.19 版起可用。
	SetupCoopTaskRun
	// SetupTaskRunFlag
	// 与 IORING_SETUP_COOP_TASKRUN 结合使用，它提供了一个标志 IORING_SQ_TASKRUN，
	// 每当有应该处理的完成等待时，它就会在 SQ 环标志中被设置。
	// 即使在执行 io_uring_peek_cqe(3) 时，uring 也会检查该标志，并进入内核处理它们，应用程序也可以这样做。
	// 这使得 IORING_SETUP_TASKRUN_FLAG 可以安全使用，即使应用程序依赖于 CQ 环上的偷看式操作来查看是否有任何待收获。自 5.19 版起可用。
	SetupTaskRunFlag
	// SetupSQE128
	// 如果设置了该选项，io_uring 将使用 128 字节的 SQE，而不是正常的 64 字节大小的 SQE。
	// 这是使用某些请求类型的要求，截至 5.19 版，只有用于 NVMe 直通的 IORING_OP_URING_CMD 直通命令需要使用此功能。自 5.19 版起可用。
	SetupSQE128
	// SetupCQE32
	// 如果设置了该选项，io_uring 将使用 32 字节的 CQE，而非通常的 16 字节大小。
	// 这是使用某些请求类型的要求，截至 5.19 版，只有用于 NVMe 直通的 IORING_OP_URING_CMD 直通命令需要使用此功能。自 5.19 版起可用。
	SetupCQE32
	// SetupSingleIssuer
	// 提示内核只有一个任务（或线程）提交请求，用于内部优化。
	// 提交任务要么是创建环的任务，要么是通过 io_uring_register(2) 启用环的任务（如果指定了 IORING_SETUP_R_DISABLED）。
	// 内核会强制执行这一规则，如果违反限制，会以 -EEXIST 失败请求。
	// 需要注意的是，当设置了 IORING_SETUP_SQPOLL 时，轮询任务将被视为代表用户空间完成所有提交工作，
	// 因此无论有多少用户空间任务执行 io_uring_enter(2)，轮询任务都会遵守该规则。
	// 自 6.0 版起可用。
	SetupSingleIssuer
	// SetupDeferTaskRun
	// 默认情况下，io_uring 会在任何系统调用或线程中断结束时处理所有未完成的工作。这可能会延迟应用程序取得其他进展。
	// 设置该标志将提示 io_uring 将工作推迟到设置了 IORING_ENTER_GETEVENTS 标志的 io_uring_enter(2) 调用。
	// 这样，应用程序就可以在处理完成之前请求运行工作。
	// 该标志要求设置 IORING_SETUP_SINGLE_ISSUER 标志，并强制要求从提交请求的同一线程调用 io_uring_enter(2)。
	// 请注意，如果设置了该标记，应用程序就有责任定期触发工作（例如通过任何 CQE 等待函数），否则可能无法交付完成。自 6.1 版起可用。
	SetupDeferTaskRun
	// SetupNoMmap
	// 默认情况下，io_uring 会分配内核内存，调用者必须随后使用 mmap(2)。
	// 如果设置了该标记，io_uring 将使用调用者分配的缓冲区；p->cq_off.user_addr 必须指向 sq/cq ring 的内存，p->sq_off.user_addr 必须指向 sqes 的内存。
	// 每次分配的内存必须是连续的。通常情况下，调用者应使用 mmap(2) 分配大页面来分配这些内存。
	// 如果设置了此标记，那么随后尝试 mmap(2) io_uring 文件描述符的操作将失败。自 6.5 版起可用。
	SetupNoMmap
	// SetupRegisteredFdOnly
	// 如果设置了这个标志，io_uring 将注册环形文件描述符，并返回已注册的描述符索引，而不会分配一个未注册的文件描述符。
	// 调用者在调用 io_uring_register(2) 时需要使用 IORING_REGISTER_USE_REGISTERED_RING。
	// 该标记只有在与 IORING_SETUP_NO_MMAP 同时使用时才有意义，后者也需要设置。自 6.5 版起可用。
	SetupRegisteredFdOnly
	// SetupNoSQArray
	// 如果设置了该标志，提交队列中的条目将按顺序提交，并在到达队列末尾后绕到第一个条目。
	// 换句话说，将不再通过提交条目数组进行间接处理，而是直接通过提交队列尾部和它所代表的索引范围（队列大小的模数）对队列进行索引。
	// 随后，用户不应映射提交队列条目数组，结构 io_sqring_offsets 中的相应偏移量将被设置为零。
	// 自 6.6 版起可用。
	SetupNoSQArray
	// SetupHybridIOPoll
	// 此标志必须与IORING_SETUP_IOPOLL标志一起使用。
	// 混合io轮询是基于iopoll的一项功能，它与严格轮询的不同之处在于，在进行完成侧轮询之前，它会延迟一点，以避免浪费太多的CPU资源。与IOPOLL一样，它要求设备支持轮询。
	SetupHybridIOPoll
)

func ParseSetupFlags(s string) uint32 {
	s = strings.TrimSpace(s)
	s = strings.ToUpper(s)
	switch s {
	case "IORING_SETUP_IOPOLL":
		return SetupIOPoll
	case "IORING_SETUP_SQPOLL":
		return SetupSQPoll
	case "IORING_SETUP_SQ_AFF":
		return SetupSQAff
	case "IORING_SETUP_CQSIZE":
		return SetupCQSize
	case "IORING_SETUP_CLAMP":
		return SetupClamp
	case "IORING_SETUP_ATTACH_WQ":
		return SetupAttachWQ
	case "IORING_SETUP_R_DISABLED":
		return SetupRDisabled
	case "IORING_SETUP_SUBMIT_ALL":
		return SetupSubmitAll
	case "IORING_SETUP_COOP_TASKRUN":
		return SetupCoopTaskRun
	case "IORING_SETUP_TASKRUN_FLAG":
		return SetupTaskRunFlag
	case "IORING_SETUP_SQE128":
		return SetupSQE128
	case "IORING_SETUP_CQE32":
		return SetupCQE32
	case "IORING_SETUP_SINGLE_ISSUER":
		return SetupSingleIssuer
	case "IORING_SETUP_DEFER_TASKRUN":
		return SetupDeferTaskRun
	case "IORING_SETUP_NO_MMAP":
		return SetupNoMmap
	case "IORING_SETUP_REGISTERED_FD_ONLY":
		return SetupRegisteredFdOnly
	case "IORING_SETUP_NO_SQARRAY":
		return SetupNoSQArray
	case "IORING_SETUP_HYBRID_IOPOLL":
		return SetupHybridIOPoll
	default:
		return 0
	}
}
