//go:build linux

package iouring

const (
	// FeatSingleMMap
	// 如果设置了该标志，则只需调用一次 mmap(2)，即可映射两个 SQ 和 CQ 环。
	// SQE 仍需单独分配。这样，所需的 mmap(2) 调用次数就从三次减少到两次。自内核 5.4 起可用。
	FeatSingleMMap uint32 = 1 << iota
	// FeatNoDrop
	// 如果设置了这个标志，io_uring 就几乎不会丢弃完成事件。只有当内核内存耗尽时，才会发生丢弃事件，
	// 在这种情况下，你会遇到比丢失事件更严重的问题。
	// 无论如何，你的应用程序和其他程序都可能会被 OOM 杀掉。
	// 如果发生了完成事件，而 CQ 环已满，内核会在内部存储该事件，直到 CQ 环有空间容纳更多条目。
	// 在早期内核中，如果出现这种溢出情况，尝试提交更多 IO 时，如果无法将溢出的事件刷新到 CQ 环，就会出现 -EBUSY 错误值而失败。
	// 如果出现这种情况，应用程序必须从 CQ 环中获取事件，并再次尝试提交。
	// 如果内核内部没有空闲内存来存储事件，那么 cqring 上的溢出值就会增加。
	// 自内核 5.5 起可用。此外，io_uring_enter(2) 还会在下一次休眠等待完成时返回 -EBADR（自内核 5.19 起）。
	FeatNoDrop
	// FeatSubmitStable
	// 如果设置了该标志，应用程序就可以确定，当内核消耗 SQE 时，任何用于异步卸载的数据都已消耗完毕。自内核 5.5 起可用。
	FeatSubmitStable
	// FeatRWCurPos
	// 如果设置了这个标志，应用程序就可以在 IORING_OP_{READV,WRITEV}、IORING_OP_{READ,WRITE}_FIXED 和 IORING_OP_{READ,WRITE} 中指定偏移量 == -1 表示当前文件位置，
	// 其行为与偏移量 == -1 的 preadv2(2) 和 pwritev2(2) 类似。
	// 它将使用（并更新）当前文件位置。
	// 这显然需要注意的是，如果应用程序在运行过程中进行了多次读取或写入，那么最终结果将不尽如人意。
	// 这与线程共享文件描述符并使用当前文件位置进行 IO 的情况类似。自内核 5.6 起可用。
	FeatRWCurPos
	// FeatCurPersonality
	// 如果设置了这个标志，那么 io_uring 将保证同步和异步执行请求时，都使用调用 io_uring_enter(2) 对请求进行排队的任务的凭据。
	// 如果未设置该标记，则会使用最初注册 io_uring 的任务的凭据发出请求。
	// 如果只有一个任务在使用一个环，那么这个标志并不重要，因为凭据始终是相同的。
	// 请注意，这是默认行为，任务仍然可以通过 io_uring_register(2) 以 IORING_REGISTER_PERSONALITY 注册不同的个性，并在 sqe 中指定要使用的个性。
	// 自内核 5.6 起可用。
	FeatCurPersonality
	// FeatFastPoll
	// 如果设置了这个标志，那么 io_uring 将支持使用内部轮询机制来驱动数据/空间就绪。
	// 这意味着无法读取或写入文件数据的请求不再需要交由异步线程处理，而是会在文件就绪时开始运行。
	// 这类似于在用户空间进行轮询 + 读/写操作，但无需这样做。
	// 如果设置了该标记，等待空间/数据的请求就不会阻塞线程，从而减少了资源消耗。自内核 5.7 起可用。
	FeatFastPoll
	// FeatPoll32Bits
	// 如果设置了该标志，IORING_OP_POLL_ADD 命令将接受全部 32 位的基于 epoll 的标志。
	// 最值得注意的是 EPOLLEXCLUSIVE，它允许独占（唤醒单个等待者）行为。自内核 5.9 起可用。
	FeatPoll32Bits
	// FeatSQPollNonfixed
	// 如果设置了该标志，IORING_SETUP_SQPOLL 功能就不再需要使用固定文件。任何普通文件描述符都可用于 IO 命令，无需注册。
	// 自内核 5.11 起可用。
	FeatSQPollNonfixed
	// FeatExtArg
	// 如果设置了这个标志，io_uring_enter(2) 系统调用就支持传递一个扩展参数，而不仅仅是早期内核的 sigset_t。
	// 这个扩展参数的类型是 struct io_uring_getevents_arg，允许调用者同时传递 sigset_t 和超时参数，以等待事件发生。
	// 结构布局如下
	// struct io_uring_getevents_arg {
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
	// 如果设置了这个标志，那么 io_uring 将使用本地 Worker 作为异步助手。
	// 以前的内核使用的内核线程会假定原始 io_uring 拥有任务的身份，但以后的内核会主动创建看起来更像普通进程的线程。
	// 自内核 5.12 起可用。
	FeatNativeWorkers
	// FeatRsrcTags
	// 如果设置了这个标志，那么 io_uring 将支持与固定文件和缓冲区相关的各种功能。
	// 尤其是，它表明已注册的缓冲区可以就地更新，而在此之前，必须先取消注册整个缓冲区。自内核 5.13 起可用。
	FeatRsrcTags
	// FeatCQESkip
	// 如果设置了该标志，io_uring 就支持在提交的 SQE 中设置 IOSQE_CQE_SKIP_SUCCESS，表明如果正常执行，就不会为该 SQE 生成 CQE。
	// 如果在处理 SQE 时发生错误，仍会生成带有相应错误值的 CQE。自内核 5.17 起可用。
	FeatCQESkip
	// FeatLinkedFile
	// 如果设置了这个标志，那么 io_uring 将支持为有依赖关系的 SQE 合理分配文件。
	// 例如，如果使用 IOSQE_IO_LINK 提交了一连串 SQE，那么没有该标志的内核将为每个链接预先准备文件。
	// 如果前一个链接打开了一个已知索引的文件，例如使用直接描述符打开或接受，那么文件分配就需要在执行该 SQE 后进行。
	// 如果设置了该标志，内核将推迟文件分配，直到开始执行给定请求。自内核 5.17 起可用。
	FeatLinkedFile
	// FeatRegRegRing
	// 如果设置了该标志，则 io_uring 支持通过 IORING_REGISTER_USE_REGISTERED_RING，使用注册环 fd 调用 io_uring_register(2)。自内核 6.3 起可用。
	FeatRegRegRing
	FeatRecvSendBundle
	// FeatMinTimeout
	// 如果设置了该标志，则 io_uring 支持传递最小批处理等待超时。详情请参见 io_uring_submit_and_wait_min_timeout(3) 。
	FeatMinTimeout
)
